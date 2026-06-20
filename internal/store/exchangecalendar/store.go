package exchangecalendar

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

type Store struct {
	root string
}

func New(root string) *Store {
	return &Store{root: strings.TrimSpace(root)}
}

func (s *Store) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}

func (s *Store) SaveSnapshot(snapshot marketcalendar.CalendarSnapshot) error {
	if s == nil {
		return errors.New("exchange calendar store is nil")
	}
	if strings.TrimSpace(s.root) == "" {
		return errors.New("exchange calendar store root is empty")
	}
	if strings.TrimSpace(snapshot.MarketCode) == "" || strings.TrimSpace(snapshot.SourceID) == "" {
		return errors.New("snapshot marketCode and sourceId are required")
	}
	year := snapshotYear(snapshot)
	if year == 0 {
		return errors.New("snapshot year is required")
	}
	path := s.snapshotPath(snapshot.MarketCode, snapshot.SourceID, year)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create exchange calendar snapshot directory: %w", err)
	}
	body, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal exchange calendar snapshot: %w", err)
	}
	return os.WriteFile(path, append(body, '\n'), 0o644)
}

func (s *Store) LoadSnapshots() ([]marketcalendar.CalendarSnapshot, []error) {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return nil, nil
	}
	var (
		snapshots []marketcalendar.CalendarSnapshot
		errs      []error
	)
	jftradeErr1 := filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			errs = append(errs, walkErr)
			return nil
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("read %s: %w", path, err))
			return nil
		}
		var snapshot marketcalendar.CalendarSnapshot
		if err := json.Unmarshal(body, &snapshot); err != nil {
			errs = append(errs, fmt.Errorf("decode %s: %w", path, err))
			return nil
		}
		snapshots = append(snapshots, snapshot)
		return nil
	})
	jftradeLogError(jftradeErr1)
	return snapshots, errs
}

func (s *Store) DeleteSnapshot(snapshot marketcalendar.CalendarSnapshot) error {
	if s == nil || strings.TrimSpace(s.root) == "" {
		return nil
	}
	year := snapshotYear(snapshot)
	if year == 0 {
		return nil
	}
	path := s.snapshotPath(snapshot.MarketCode, snapshot.SourceID, year)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete exchange calendar snapshot %s: %w", path, err)
	}
	return nil
}

func (s *Store) snapshotPath(marketCode string, sourceID string, year int) string {
	return filepath.Join(s.root, strings.ToUpper(strings.TrimSpace(marketCode)), fmt.Sprintf("%04d", year), strings.TrimSpace(sourceID)+".json")
}

func snapshotYear(snapshot marketcalendar.CalendarSnapshot) int {
	switch {
	case !snapshot.From.IsZero():
		return snapshot.From.Year()
	case !snapshot.To.IsZero():
		return snapshot.To.Year()
	case len(snapshot.Schedules) > 0:
		return snapshot.Schedules[0].Date.Year()
	default:
		return 0
	}
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
