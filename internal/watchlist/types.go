package watchlist

import (
	"strings"
	"time"
)

const (
	DefaultGroupName = "自选股"
	DefaultPageLimit = 100
	MaxPageLimit     = 500
)

// GroupNameKey returns the canonical uniqueness key shared by the watchlist
// domain and its persistent store.
func GroupNameKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// Group is a JFTrade-owned watchlist group.
type Group struct {
	ID        string    `json:"groupId" db:"group_id"`
	Name      string    `json:"name" db:"name"`
	IsDefault bool      `json:"isDefault" db:"is_default"`
	Protected bool      `json:"protected" db:"protected"`
	Revision  int64     `json:"revision" db:"revision"`
	ItemCount int       `json:"itemCount" db:"item_count"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}

type CreateGroupInput struct {
	Name string `json:"name" binding:"required"`
}

type UpdateGroupInput struct {
	Name             string `json:"name" binding:"required"`
	ExpectedRevision int64  `json:"expectedRevision" binding:"required,min=1"`
}

type Instrument struct {
	ID             string     `json:"instrumentId" db:"instrument_id"`
	Market         string     `json:"market" db:"market"`
	Symbol         string     `json:"symbol" db:"symbol"`
	Name           string     `json:"name,omitempty" db:"name"`
	Type           string     `json:"type,omitempty" db:"instrument_type"`
	Revision       int64      `json:"revision" db:"membership_revision"`
	SourceIDs      []string   `json:"sourceIds,omitempty"`
	GroupIDs       []string   `json:"groupIds"`
	LastImportedAt *time.Time `json:"lastImportedAt,omitempty"`
}

type Item struct {
	Instrument
	Groups []GroupRef `json:"groups"`
}

type GroupRef struct {
	ID   string `json:"groupId" db:"group_id"`
	Name string `json:"name" db:"name"`
}

type ListItemsOptions struct {
	GroupID string
	Cursor  string
	Limit   int
	Query   string
	Market  string
}

type ItemPage struct {
	Items      []Item `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type Memberships struct {
	InstrumentID string     `json:"instrumentId"`
	Revision     int64      `json:"revision"`
	Groups       []GroupRef `json:"groups"`
}

type ReplaceMembershipsInput struct {
	InstrumentID     string   `json:"-"`
	GroupIDs         []string `json:"groupIds"`
	NewGroupNames    []string `json:"newGroupNames"`
	ExpectedRevision int64    `json:"expectedRevision"`
}

type Source struct {
	ID          string    `json:"sourceId" db:"source_id"`
	Broker      string    `json:"broker" db:"broker"`
	DisplayName string    `json:"displayName" db:"display_name"`
	Status      string    `json:"status" db:"status"`
	Error       string    `json:"error,omitempty" db:"last_error"`
	UpdatedAt   time.Time `json:"updatedAt" db:"updated_at"`
}

type RemoteGroup struct {
	SourceID      string    `json:"sourceId" db:"source_id"`
	RemoteGroupID string    `json:"remoteGroupId" db:"remote_group_id"`
	Name          string    `json:"name" db:"name"`
	Type          string    `json:"type" db:"group_type"`
	Ambiguous     bool      `json:"ambiguous" db:"ambiguous"`
	MemberCount   int       `json:"memberCount" db:"member_count"`
	RemoteHash    string    `json:"remoteHash,omitempty" db:"remote_hash"`
	ObservedAt    time.Time `json:"observedAt" db:"observed_at"`
}

type RemoteMember struct {
	InstrumentID string `json:"instrumentId"`
	Name         string `json:"name,omitempty"`
	Type         string `json:"type,omitempty"`
	BrokerCode   string `json:"brokerCode,omitempty"`
	SecurityID   string `json:"securityId,omitempty"`
}

type Binding struct {
	ID            string    `json:"bindingId" db:"binding_id"`
	SourceID      string    `json:"sourceId" db:"source_id"`
	RemoteGroupID string    `json:"remoteGroupId" db:"remote_group_id"`
	RemoteName    string    `json:"remoteName" db:"remote_name"`
	LocalGroupID  string    `json:"localGroupId" db:"local_group_id"`
	CreatedAt     time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt     time.Time `json:"updatedAt" db:"updated_at"`
}

type ImportPreviewRequest struct {
	SourceID      string `json:"sourceId" binding:"required"`
	RemoteGroupID string `json:"remoteGroupId" binding:"required"`
	LocalGroupID  string `json:"localGroupId,omitempty"`
	NewGroupName  string `json:"newGroupName,omitempty"`
}

type ImportDiffItem struct {
	InstrumentID string `json:"instrumentId"`
	Name         string `json:"name,omitempty"`
	Type         string `json:"type,omitempty"`
	Selected     bool   `json:"selected"`
}

type ImportPreview struct {
	ID                 string           `json:"previewId" db:"preview_id"`
	SourceID           string           `json:"sourceId" db:"source_id"`
	RemoteGroupID      string           `json:"remoteGroupId" db:"remote_group_id"`
	RemoteGroupName    string           `json:"remoteGroupName" db:"remote_group_name"`
	LocalGroupID       string           `json:"localGroupId,omitempty" db:"local_group_id"`
	NewGroupName       string           `json:"newGroupName,omitempty" db:"new_group_name"`
	RemoteHash         string           `json:"remoteHash" db:"remote_hash"`
	LocalGroupRevision int64            `json:"localGroupRevision" db:"local_group_revision"`
	Added              []ImportDiffItem `json:"added"`
	Unchanged          []ImportDiffItem `json:"unchanged"`
	LocalOnly          []ImportDiffItem `json:"localOnly"`
	CreatedAt          time.Time        `json:"createdAt" db:"created_at"`
	ExpiresAt          time.Time        `json:"expiresAt" db:"expires_at"`
}

type CommitImportInput struct {
	PreviewID           string   `json:"-"`
	DeleteInstrumentIDs []string `json:"deleteInstrumentIds"`
}

type ImportRun struct {
	ID              string    `json:"runId" db:"run_id"`
	PreviewID       string    `json:"previewId" db:"preview_id"`
	SourceID        string    `json:"sourceId" db:"source_id"`
	RemoteGroupID   string    `json:"remoteGroupId" db:"remote_group_id"`
	RemoteGroupName string    `json:"remoteGroupName" db:"remote_group_name"`
	LocalGroupID    string    `json:"localGroupId" db:"local_group_id"`
	Status          string    `json:"status" db:"status"`
	AddedCount      int       `json:"addedCount" db:"added_count"`
	RemovedCount    int       `json:"removedCount" db:"removed_count"`
	UnchangedCount  int       `json:"unchangedCount" db:"unchanged_count"`
	RemoteHash      string    `json:"remoteHash" db:"remote_hash"`
	CreatedAt       time.Time `json:"createdAt" db:"created_at"`
	CompletedAt     time.Time `json:"completedAt" db:"completed_at"`
}

type ImportRunPage struct {
	Items      []ImportRun `json:"items"`
	NextCursor string      `json:"nextCursor,omitempty"`
}

type CommitImportStoreInput struct {
	Preview             ImportPreview
	RemoteMembers       []RemoteMember
	DeleteInstrumentIDs []string
}

// InstrumentMetadata is non-authoritative display metadata learned from a
// broker snapshot. It never participates in membership identity or revision.
type InstrumentMetadata struct {
	InstrumentID string
	Name         string
	Type         string
}

type Quote struct {
	InstrumentID  string         `json:"instrumentId"`
	Name          string         `json:"name,omitempty"`
	Type          string         `json:"type,omitempty"`
	Source        string         `json:"source,omitempty"`
	Price         *float64       `json:"price,omitempty"`
	PreviousClose *float64       `json:"previousClose,omitempty"`
	Change        *float64       `json:"change,omitempty"`
	ChangePercent *float64       `json:"changePercent,omitempty"`
	Session       string         `json:"session,omitempty"`
	ObservedAt    time.Time      `json:"observedAt"`
	UpdateTime    *time.Time     `json:"updateTime,omitempty"`
	Extended      *ExtendedQuote `json:"extended,omitempty"`
}

type ExtendedQuote struct {
	Pre       *QuoteBlock `json:"pre,omitempty"`
	After     *QuoteBlock `json:"after,omitempty"`
	Overnight *QuoteBlock `json:"overnight,omitempty"`
}

type QuoteBlock struct {
	Price         *float64   `json:"price,omitempty"`
	Change        *float64   `json:"change,omitempty"`
	ChangePercent *float64   `json:"changePercent,omitempty"`
	ObservedAt    time.Time  `json:"observedAt"`
	UpdateTime    *time.Time `json:"updateTime,omitempty"`
}

type QuoteError struct {
	InstrumentID string `json:"instrumentId"`
	Code         string `json:"code"`
	Message      string `json:"message"`
}

type BatchQuotes struct {
	Quotes     []Quote      `json:"quotes"`
	Errors     []QuoteError `json:"errors"`
	ObservedAt time.Time    `json:"observedAt"`
}
