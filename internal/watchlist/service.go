package watchlist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jftrade/jftrade-main/pkg/market"
)

const (
	defaultPreviewTTL = 10 * time.Minute
	defaultQuoteTTL   = 2500 * time.Millisecond
)

type Option func(*Service)

func WithClock(clock func() time.Time) Option {
	return func(service *Service) {
		if clock != nil {
			service.now = clock
		}
	}
}

func WithPreviewTTL(ttl time.Duration) Option {
	return func(service *Service) {
		if ttl > 0 {
			service.previewTTL = ttl
		}
	}
}

func WithQuoteCacheTTL(ttl time.Duration) Option {
	return func(service *Service) {
		if ttl > 0 {
			service.quoteTTL = ttl
		}
	}
}

func WithSourceReader(sourceID string, reader WatchlistSourceReader) Option {
	return func(service *Service) { service.RegisterSourceReader(sourceID, reader) }
}

func WithBatchSnapshotSource(source BatchSnapshotSource) Option {
	return func(service *Service) { service.RegisterBatchSnapshotSource(source) }
}

type Service struct {
	repository  Repository
	now         func() time.Time
	previewTTL  time.Duration
	mu          sync.RWMutex
	readers     map[string]WatchlistSourceReader
	quoteSource BatchSnapshotSource
	quoteTTL    time.Duration
	quoteMu     sync.Mutex
	quoteCache  map[string]quoteCacheEntry
	quoteFlight map[string]*quoteFlight
}

type quoteCacheEntry struct {
	quote     *Quote
	itemError *QuoteError
	expiresAt time.Time
}

type quoteFlight struct{ done chan struct{} }

func NewService(repository Repository, options ...Option) *Service {
	service := &Service{
		repository:  repository,
		now:         func() time.Time { return time.Now().UTC() },
		previewTTL:  defaultPreviewTTL,
		readers:     make(map[string]WatchlistSourceReader),
		quoteTTL:    defaultQuoteTTL,
		quoteCache:  make(map[string]quoteCacheEntry),
		quoteFlight: make(map[string]*quoteFlight),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) Available() bool { return s != nil && s.repository != nil }

func (s *Service) RegisterSourceReader(sourceID string, reader WatchlistSourceReader) {
	if s == nil || reader == nil || strings.TrimSpace(sourceID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readers[strings.TrimSpace(sourceID)] = reader
}

func (s *Service) RegisterBatchSnapshotSource(source BatchSnapshotSource) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quoteSource = source
}

func (s *Service) ListGroups(ctx context.Context) ([]Group, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	return s.repository.ListGroups(ctx)
}

func (s *Service) CreateGroup(ctx context.Context, input CreateGroupInput) (Group, error) {
	if !s.Available() {
		return Group{}, ErrUnavailable
	}
	name, err := normalizeGroupName(input.Name)
	if err != nil {
		return Group{}, err
	}
	return s.repository.CreateGroup(ctx, name)
}

func (s *Service) UpdateGroup(ctx context.Context, groupID string, input UpdateGroupInput) (Group, error) {
	if !s.Available() {
		return Group{}, ErrUnavailable
	}
	if strings.TrimSpace(groupID) == "" || input.ExpectedRevision < 1 {
		return Group{}, fmt.Errorf("%w: groupId and expectedRevision are required", ErrValidation)
	}
	name, err := normalizeGroupName(input.Name)
	if err != nil {
		return Group{}, err
	}
	return s.repository.UpdateGroup(ctx, strings.TrimSpace(groupID), name, input.ExpectedRevision)
}

func (s *Service) DeleteGroup(ctx context.Context, groupID string) error {
	if !s.Available() {
		return ErrUnavailable
	}
	if strings.TrimSpace(groupID) == "" {
		return fmt.Errorf("%w: groupId is required", ErrValidation)
	}
	return s.repository.DeleteGroup(ctx, strings.TrimSpace(groupID))
}

func (s *Service) ListItems(ctx context.Context, options ListItemsOptions) (ItemPage, error) {
	if !s.Available() {
		return ItemPage{}, ErrUnavailable
	}
	options.GroupID = strings.TrimSpace(options.GroupID)
	options.Cursor = strings.TrimSpace(options.Cursor)
	options.Query = strings.TrimSpace(options.Query)
	options.Market = strings.ToUpper(strings.TrimSpace(options.Market))
	options.Limit = normalizeLimit(options.Limit)
	if options.Market != "" {
		resolved, preferred, err := market.NormalizeMarketInput(options.Market)
		if err != nil {
			return ItemPage{}, fmt.Errorf("%w: %v", ErrValidation, err)
		}
		if preferred != "" {
			options.Market = preferred
		} else {
			options.Market = resolved
		}
	}
	return s.repository.ListItems(ctx, options)
}

func (s *Service) GetMemberships(ctx context.Context, instrumentID string) (Memberships, error) {
	if !s.Available() {
		return Memberships{}, ErrUnavailable
	}
	normalized, err := NormalizeInstrumentID(instrumentID)
	if err != nil {
		return Memberships{}, err
	}
	return s.repository.GetMemberships(ctx, normalized)
}

func (s *Service) ReplaceMemberships(ctx context.Context, input ReplaceMembershipsInput) (Memberships, error) {
	if !s.Available() {
		return Memberships{}, ErrUnavailable
	}
	normalized, err := NormalizeInstrumentID(input.InstrumentID)
	if err != nil {
		return Memberships{}, err
	}
	input.InstrumentID = normalized
	input.GroupIDs = normalizedUnique(input.GroupIDs)
	for index, name := range input.NewGroupNames {
		input.NewGroupNames[index], err = normalizeGroupName(name)
		if err != nil {
			return Memberships{}, err
		}
	}
	input.NewGroupNames = normalizedUniqueFold(input.NewGroupNames)
	if input.ExpectedRevision < 0 {
		return Memberships{}, fmt.Errorf("%w: expectedRevision cannot be negative", ErrValidation)
	}
	return s.repository.ReplaceMemberships(ctx, input)
}

func (s *Service) ListSources(ctx context.Context) ([]Source, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	s.mu.RLock()
	readers := make(map[string]WatchlistSourceReader, len(s.readers))
	for id, reader := range s.readers {
		readers[id] = reader
	}
	s.mu.RUnlock()
	for id, reader := range readers {
		source, err := reader.Source(ctx)
		if strings.TrimSpace(source.ID) == "" {
			source.ID = id
		}
		source.ID = id
		source.UpdatedAt = s.now()
		if err != nil {
			source.Status = "unavailable"
			source.Error = err.Error()
		} else if strings.TrimSpace(source.Status) == "" {
			source.Status = "ready"
		}
		if upsertErr := s.repository.UpsertSource(ctx, source); upsertErr != nil {
			return nil, upsertErr
		}
	}
	return s.repository.ListSources(ctx)
}

func (s *Service) ListSourceGroups(ctx context.Context, sourceID string) ([]RemoteGroup, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	sourceID = strings.TrimSpace(sourceID)
	reader, err := s.sourceReader(sourceID)
	if err != nil {
		return nil, err
	}
	groups, err := reader.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	if groups == nil {
		groups = []RemoteGroup{}
	}
	now := s.now()
	nameCounts := make(map[string]int, len(groups))
	for _, group := range groups {
		nameCounts[GroupNameKey(group.Name)]++
	}
	for index := range groups {
		groups[index].SourceID = strings.TrimSpace(sourceID)
		groups[index].Name = strings.TrimSpace(groups[index].Name)
		groups[index].Ambiguous = groups[index].Ambiguous || nameCounts[GroupNameKey(groups[index].Name)] > 1
		if groups[index].ObservedAt.IsZero() {
			groups[index].ObservedAt = now
		}
	}
	if err := s.repository.ReplaceRemoteGroups(ctx, sourceID, groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *Service) ListBindings(ctx context.Context, sourceID string) ([]Binding, error) {
	if !s.Available() {
		return nil, ErrUnavailable
	}
	return s.repository.ListBindings(ctx, strings.TrimSpace(sourceID))
}

func (s *Service) DeleteBinding(ctx context.Context, bindingID string) error {
	if !s.Available() {
		return ErrUnavailable
	}
	if strings.TrimSpace(bindingID) == "" {
		return fmt.Errorf("%w: bindingId is required", ErrValidation)
	}
	return s.repository.DeleteBinding(ctx, strings.TrimSpace(bindingID))
}

func (s *Service) PreviewImport(ctx context.Context, request ImportPreviewRequest) (ImportPreview, error) {
	if !s.Available() {
		return ImportPreview{}, ErrUnavailable
	}
	request, err := normalizePreviewRequest(request)
	if err != nil {
		return ImportPreview{}, err
	}
	reader, err := s.sourceReader(request.SourceID)
	if err != nil {
		return ImportPreview{}, err
	}
	groups, err := s.ListSourceGroups(ctx, request.SourceID)
	if err != nil {
		return ImportPreview{}, err
	}
	remoteGroup, ok := findRemoteGroup(groups, request.RemoteGroupID)
	if !ok {
		return ImportPreview{}, ErrNotFound
	}
	if remoteGroup.Ambiguous {
		return ImportPreview{}, ErrAmbiguousRemoteGroup
	}
	if request.LocalGroupID == "" && request.NewGroupName == "" {
		request.NewGroupName = remoteGroup.Name
	}
	if request.NewGroupName != "" {
		request.NewGroupName, err = normalizeGroupName(request.NewGroupName)
		if err != nil {
			return ImportPreview{}, err
		}
	}
	remoteMembers, err := reader.ListGroupMembers(ctx, request.RemoteGroupID)
	if err != nil {
		return ImportPreview{}, err
	}
	remoteMembers, err = normalizeRemoteMembers(remoteMembers)
	if err != nil {
		return ImportPreview{}, err
	}
	localIDs := []string(nil)
	localRevision := int64(0)
	if request.LocalGroupID != "" {
		localGroup, getErr := s.repository.GetGroup(ctx, request.LocalGroupID)
		if getErr != nil {
			return ImportPreview{}, getErr
		}
		localRevision = localGroup.Revision
		localIDs, err = s.repository.GroupInstrumentIDs(ctx, request.LocalGroupID)
		if err != nil {
			return ImportPreview{}, err
		}
	}
	added, unchanged, localOnly := diffMembers(remoteMembers, localIDs)
	now := s.now()
	preview := ImportPreview{
		ID:                 newID("wlpreview"),
		SourceID:           request.SourceID,
		RemoteGroupID:      request.RemoteGroupID,
		RemoteGroupName:    remoteGroup.Name,
		LocalGroupID:       request.LocalGroupID,
		NewGroupName:       request.NewGroupName,
		RemoteHash:         hashMembers(remoteMembers),
		LocalGroupRevision: localRevision,
		Added:              added,
		Unchanged:          unchanged,
		LocalOnly:          localOnly,
		CreatedAt:          now,
		ExpiresAt:          now.Add(s.previewTTL),
	}
	if err := s.repository.SaveImportPreview(ctx, preview); err != nil {
		return ImportPreview{}, err
	}
	return preview, nil
}

func normalizePreviewRequest(request ImportPreviewRequest) (ImportPreviewRequest, error) {
	request.SourceID = strings.TrimSpace(request.SourceID)
	request.RemoteGroupID = strings.TrimSpace(request.RemoteGroupID)
	request.LocalGroupID = strings.TrimSpace(request.LocalGroupID)
	request.NewGroupName = strings.TrimSpace(request.NewGroupName)
	if request.SourceID == "" || request.RemoteGroupID == "" {
		return ImportPreviewRequest{}, fmt.Errorf("%w: sourceId and remoteGroupId are required", ErrValidation)
	}
	if request.LocalGroupID != "" && request.NewGroupName != "" {
		return ImportPreviewRequest{}, fmt.Errorf("%w: choose localGroupId or newGroupName", ErrValidation)
	}
	return request, nil
}

func (s *Service) CommitImport(ctx context.Context, input CommitImportInput) (ImportRun, error) {
	if !s.Available() {
		return ImportRun{}, ErrUnavailable
	}
	input.PreviewID = strings.TrimSpace(input.PreviewID)
	if input.PreviewID == "" {
		return ImportRun{}, fmt.Errorf("%w: previewId is required", ErrValidation)
	}
	preview, err := s.repository.GetImportPreview(ctx, input.PreviewID)
	if err != nil {
		return ImportRun{}, err
	}
	if !s.now().Before(preview.ExpiresAt) {
		return ImportRun{}, ErrPreviewExpired
	}
	if preview.LocalGroupID != "" {
		group, getErr := s.repository.GetGroup(ctx, preview.LocalGroupID)
		if getErr != nil || group.Revision != preview.LocalGroupRevision {
			return ImportRun{}, ErrStalePreview
		}
	}
	reader, err := s.sourceReader(preview.SourceID)
	if err != nil {
		return ImportRun{}, err
	}
	remoteMembers, err := freshGroupMembers(ctx, reader, preview.RemoteGroupID)
	if err != nil {
		return ImportRun{}, fmt.Errorf("%w: remote group revalidation failed: %v", ErrStalePreview, err)
	}
	remoteMembers, err = normalizeRemoteMembers(remoteMembers)
	if err != nil {
		return ImportRun{}, err
	}
	if hashMembers(remoteMembers) != preview.RemoteHash {
		return ImportRun{}, ErrStalePreview
	}
	if !s.now().Before(preview.ExpiresAt) {
		return ImportRun{}, ErrPreviewExpired
	}
	allowedDeletes := make(map[string]struct{}, len(preview.LocalOnly))
	for _, item := range preview.LocalOnly {
		allowedDeletes[item.InstrumentID] = struct{}{}
	}
	deleteInstrumentIDs := make([]string, 0, len(input.DeleteInstrumentIDs))
	for _, value := range input.DeleteInstrumentIDs {
		instrumentID, normalizeErr := NormalizeInstrumentID(value)
		if normalizeErr != nil {
			return ImportRun{}, normalizeErr
		}
		deleteInstrumentIDs = append(deleteInstrumentIDs, instrumentID)
	}
	input.DeleteInstrumentIDs = normalizedUnique(deleteInstrumentIDs)
	for _, instrumentID := range input.DeleteInstrumentIDs {
		if _, ok := allowedDeletes[instrumentID]; !ok {
			return ImportRun{}, fmt.Errorf("%w: %s is not local-only in this preview", ErrValidation, instrumentID)
		}
	}
	return s.repository.CommitImport(ctx, CommitImportStoreInput{
		Preview:             preview,
		RemoteMembers:       remoteMembers,
		DeleteInstrumentIDs: input.DeleteInstrumentIDs,
	})
}

func freshGroupMembers(ctx context.Context, reader WatchlistSourceReader, remoteGroupID string) ([]RemoteMember, error) {
	freshReader, ok := reader.(FreshWatchlistSourceReader)
	if !ok {
		return nil, fmt.Errorf("%w: source connector does not support fresh watchlist reads", ErrUnavailable)
	}
	return freshReader.ListGroupMembersFresh(ctx, remoteGroupID)
}

func (s *Service) ListImportRuns(ctx context.Context, sourceID, cursor string, limit int) (ImportRunPage, error) {
	if !s.Available() {
		return ImportRunPage{}, ErrUnavailable
	}
	return s.repository.ListImportRuns(ctx, strings.TrimSpace(sourceID), strings.TrimSpace(cursor), normalizeLimit(limit))
}

func (s *Service) BatchQuotes(ctx context.Context, instrumentIDs []string) (BatchQuotes, error) {
	if s == nil {
		return BatchQuotes{}, ErrUnavailable
	}
	s.mu.RLock()
	source := s.quoteSource
	s.mu.RUnlock()
	if source == nil {
		return BatchQuotes{}, ErrUnavailable
	}
	if len(instrumentIDs) == 0 || len(instrumentIDs) > MaxPageLimit {
		return BatchQuotes{}, fmt.Errorf("%w: instrumentIds must contain 1-%d items", ErrValidation, MaxPageLimit)
	}
	normalized := make([]string, 0, len(instrumentIDs))
	seen := make(map[string]struct{}, len(instrumentIDs))
	for _, value := range instrumentIDs {
		instrumentID, err := NormalizeInstrumentID(value)
		if err != nil {
			return BatchQuotes{}, err
		}
		if _, ok := seen[instrumentID]; ok {
			continue
		}
		seen[instrumentID] = struct{}{}
		normalized = append(normalized, instrumentID)
	}
	owned, waitFor := s.reserveQuoteFlights(normalized)
	if len(owned) > 0 {
		quotes, itemErrors, err := source.BatchSnapshots(ctx, owned)
		s.updateInstrumentMetadata(ctx, quotes)
		s.completeQuoteFlights(owned, quotes, itemErrors, err)
	}
	for _, flight := range waitFor {
		select {
		case <-ctx.Done():
			return BatchQuotes{}, ctx.Err()
		case <-flight.done:
		}
	}
	quotes, itemErrors := s.collectQuoteCache(normalized)
	return BatchQuotes{Quotes: nonNilQuotes(quotes), Errors: nonNilQuoteErrors(itemErrors), ObservedAt: s.now()}, nil
}

func (s *Service) updateInstrumentMetadata(ctx context.Context, quotes []Quote) {
	writer, ok := s.repository.(InstrumentMetadataWriter)
	if !ok || len(quotes) == 0 {
		return
	}
	metadata := make([]InstrumentMetadata, 0, len(quotes))
	for _, quote := range quotes {
		if strings.TrimSpace(quote.Name) == "" && strings.TrimSpace(quote.Type) == "" {
			continue
		}
		metadata = append(metadata, InstrumentMetadata{
			InstrumentID: quote.InstrumentID,
			Name:         strings.TrimSpace(quote.Name),
			Type:         strings.TrimSpace(quote.Type),
		})
	}
	if len(metadata) > 0 {
		_ = writer.UpdateInstrumentMetadata(ctx, metadata)
	}
}

func (s *Service) reserveQuoteFlights(instrumentIDs []string) ([]string, []*quoteFlight) {
	s.quoteMu.Lock()
	defer s.quoteMu.Unlock()
	now := s.now()
	owned := make([]string, 0, len(instrumentIDs))
	waitFor := make([]*quoteFlight, 0)
	seenFlights := make(map[*quoteFlight]struct{})
	for _, instrumentID := range instrumentIDs {
		if entry, ok := s.quoteCache[instrumentID]; ok && now.Before(entry.expiresAt) {
			continue
		}
		delete(s.quoteCache, instrumentID)
		if flight := s.quoteFlight[instrumentID]; flight != nil {
			if _, seen := seenFlights[flight]; !seen {
				waitFor = append(waitFor, flight)
				seenFlights[flight] = struct{}{}
			}
			continue
		}
		flight := &quoteFlight{done: make(chan struct{})}
		s.quoteFlight[instrumentID] = flight
		owned = append(owned, instrumentID)
	}
	return owned, waitFor
}

func (s *Service) completeQuoteFlights(instrumentIDs []string, quotes []Quote, itemErrors []QuoteError, batchErr error) {
	quoteByID := make(map[string]Quote, len(quotes))
	for _, quote := range quotes {
		quoteByID[quote.InstrumentID] = quote
	}
	errorByID := make(map[string]QuoteError, len(itemErrors))
	for _, itemError := range itemErrors {
		errorByID[itemError.InstrumentID] = itemError
	}
	s.quoteMu.Lock()
	defer s.quoteMu.Unlock()
	expiresAt := s.now().Add(s.quoteTTL)
	for _, instrumentID := range instrumentIDs {
		entry := quoteCacheEntry{expiresAt: expiresAt}
		if quote, ok := quoteByID[instrumentID]; ok {
			copy := quote
			entry.quote = &copy
		} else if itemError, ok := errorByID[instrumentID]; ok {
			copy := itemError
			entry.itemError = &copy
		} else {
			message := "snapshot source returned no result"
			code := "NO_SNAPSHOT"
			if batchErr != nil {
				message, code = batchErr.Error(), "SNAPSHOT_FAILED"
			}
			entry.itemError = &QuoteError{InstrumentID: instrumentID, Code: code, Message: message}
		}
		s.quoteCache[instrumentID] = entry
		if flight := s.quoteFlight[instrumentID]; flight != nil {
			delete(s.quoteFlight, instrumentID)
			close(flight.done)
		}
	}
}

func (s *Service) collectQuoteCache(instrumentIDs []string) ([]Quote, []QuoteError) {
	s.quoteMu.Lock()
	defer s.quoteMu.Unlock()
	now := s.now()
	quotes := make([]Quote, 0, len(instrumentIDs))
	itemErrors := make([]QuoteError, 0)
	for _, instrumentID := range instrumentIDs {
		entry, ok := s.quoteCache[instrumentID]
		if !ok || !now.Before(entry.expiresAt) {
			itemErrors = append(itemErrors, QuoteError{InstrumentID: instrumentID, Code: "NO_SNAPSHOT", Message: "snapshot result is unavailable"})
			continue
		}
		if entry.quote != nil {
			quotes = append(quotes, *entry.quote)
		}
		if entry.itemError != nil {
			itemErrors = append(itemErrors, *entry.itemError)
		}
	}
	return quotes, itemErrors
}

func (s *Service) sourceReader(sourceID string) (WatchlistSourceReader, error) {
	if s == nil {
		return nil, ErrUnavailable
	}
	s.mu.RLock()
	reader := s.readers[strings.TrimSpace(sourceID)]
	s.mu.RUnlock()
	if reader == nil {
		return nil, ErrNotFound
	}
	return reader, nil
}

func NormalizeInstrumentID(value string) (string, error) {
	parsed, err := market.ParseInstrument(market.InstrumentInput{InstrumentID: value})
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return parsed.Symbol, nil
}

func normalizeGroupName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%w: group name is required", ErrValidation)
	}
	if len([]rune(value)) > 64 {
		return "", fmt.Errorf("%w: group name must not exceed 64 characters", ErrValidation)
	}
	return value, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return DefaultPageLimit
	}
	if limit > MaxPageLimit {
		return MaxPageLimit
	}
	return limit
}

func normalizedUnique(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func normalizedUniqueFold(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := GroupNameKey(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, strings.TrimSpace(value))
	}
	return result
}

func findRemoteGroup(groups []RemoteGroup, remoteGroupID string) (RemoteGroup, bool) {
	for _, group := range groups {
		if group.RemoteGroupID == remoteGroupID {
			return group, true
		}
	}
	return RemoteGroup{}, false
}

func normalizeRemoteMembers(members []RemoteMember) ([]RemoteMember, error) {
	byID := make(map[string]RemoteMember, len(members))
	for _, member := range members {
		instrumentID, err := NormalizeInstrumentID(member.InstrumentID)
		if err != nil {
			return nil, err
		}
		member.InstrumentID = instrumentID
		member.Name = strings.TrimSpace(member.Name)
		member.Type = strings.TrimSpace(member.Type)
		byID[instrumentID] = member
	}
	result := make([]RemoteMember, 0, len(byID))
	for _, member := range byID {
		result = append(result, member)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].InstrumentID < result[j].InstrumentID })
	return result, nil
}

func diffMembers(remoteMembers []RemoteMember, localIDs []string) ([]ImportDiffItem, []ImportDiffItem, []ImportDiffItem) {
	remote := make(map[string]RemoteMember, len(remoteMembers))
	for _, member := range remoteMembers {
		remote[member.InstrumentID] = member
	}
	local := make(map[string]struct{}, len(localIDs))
	for _, instrumentID := range localIDs {
		local[instrumentID] = struct{}{}
	}
	added := make([]ImportDiffItem, 0)
	unchanged := make([]ImportDiffItem, 0)
	for _, member := range remoteMembers {
		item := ImportDiffItem{InstrumentID: member.InstrumentID, Name: member.Name, Type: member.Type, Selected: true}
		if _, ok := local[member.InstrumentID]; ok {
			item.Selected = false
			unchanged = append(unchanged, item)
		} else {
			added = append(added, item)
		}
	}
	localOnly := make([]ImportDiffItem, 0)
	for instrumentID := range local {
		if _, ok := remote[instrumentID]; !ok {
			localOnly = append(localOnly, ImportDiffItem{InstrumentID: instrumentID, Selected: false})
		}
	}
	sort.Slice(localOnly, func(i, j int) bool { return localOnly[i].InstrumentID < localOnly[j].InstrumentID })
	return added, unchanged, localOnly
}

func hashMembers(members []RemoteMember) string {
	ids := make([]string, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.InstrumentID)
	}
	sort.Strings(ids)
	digest := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	return hex.EncodeToString(digest[:])
}

func newID(prefix string) string {
	return prefix + "_" + uuid.NewString()
}

func nonNilQuotes(values []Quote) []Quote {
	if values == nil {
		return []Quote{}
	}
	return values
}

func nonNilQuoteErrors(values []QuoteError) []QuoteError {
	if values == nil {
		return []QuoteError{}
	}
	return values
}

func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict) || errors.Is(err, ErrStalePreview) || errors.Is(err, ErrPreviewExpired)
}
