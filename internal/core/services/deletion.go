package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// ErrInvalidDeletion is returned for a deletion the API cannot express: an
// unknown target, or a record target without an ID.
var ErrInvalidDeletion = errors.New("invalid deletion")

// ErrNothingToDelete is returned when the record named does not exist.
var ErrNothingToDelete = errors.New("nothing to delete")

// ErrDeletionRefused is returned for a deletion the rules forbid, with the
// reason wrapped around it.
var ErrDeletionRefused = errors.New("this cannot be deleted")

// ErrDeletionUnconfirmed is returned for a deletion sent without the token
// of the preview the user confirmed.
var ErrDeletionUnconfirmed = errors.New("a deletion needs the token of the preview that was confirmed")

// ErrReachChanged is returned, inside a ReachChangedError, when what a
// deletion would reach is no longer what the user was shown.
var ErrReachChanged = errors.New("what this deletion reaches has changed since it was confirmed")

// ReachChangedError carries the reach as it is now, so the client can show
// it and ask again.
type ReachChangedError struct {
	Reach *domain.DeletionReach
}

func (e *ReachChangedError) Error() string { return ErrReachChanged.Error() }

// Unwrap lets errors.Is match ErrReachChanged.
func (e *ReachChangedError) Unwrap() error { return ErrReachChanged }

// deletionCredentials is what "delete all data" needs of the credential
// service: which keys exist, and a way to delete each (keychain first, then
// its name, ADR-0019).
type deletionCredentials interface {
	List(ctx context.Context) ([]domain.CredentialRef, error)
	Delete(ctx context.Context, source, label string) error
	FromEnvironment(ref domain.CredentialRef) bool
}

// DeletionService carries out every deletion
// docs/02_features/15_data_deletion.md lists, as ADR-0021 describes: what
// a deletion reaches is worked out here from domain.DeletionRules, shown to
// the user, and deleted only with the token of what they were shown.
type DeletionService struct {
	store       ports.DeletionStore
	credentials deletionCredentials
	// browserProfile and log are what "delete all data" removes outside the
	// database, before and after it respectively (ADR-0021 §7). Either may
	// be nil where there is none, as in the CLI's tests.
	browserProfile ports.LocalDataEraser
	log            ports.LocalDataEraser
}

// NewDeletionService constructs a DeletionService.
func NewDeletionService(store ports.DeletionStore, credentials deletionCredentials,
	browserProfile, log ports.LocalDataEraser) *DeletionService {
	return &DeletionService{store: store, credentials: credentials, browserProfile: browserProfile, log: log}
}

// Preview returns what deleting target would take with it, and the token
// that deleting it requires.
func (s *DeletionService) Preview(ctx context.Context, target domain.DeletionTarget, id string) (*domain.DeletionReach, error) {
	if err := validateDeletion(target, id); err != nil {
		return nil, err
	}
	if target == domain.DeleteEverything {
		return s.everythingReach(ctx)
	}

	var reach *domain.DeletionReach
	err := s.store.InTransaction(ctx, func(tx ports.DeletionTx) error {
		var err error
		reach, _, _, err = s.reach(ctx, tx, target, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return reach, nil
}

// Delete deletes target, provided token is the one its preview returned
// and what it reaches has not changed since.
func (s *DeletionService) Delete(ctx context.Context, target domain.DeletionTarget, id, token string) (*domain.DeletionResult, error) {
	if err := validateDeletion(target, id); err != nil {
		return nil, err
	}
	if token == "" {
		return nil, ErrDeletionUnconfirmed
	}
	if target == domain.DeleteEverything {
		return s.deleteEverything(ctx, token)
	}

	var reach *domain.DeletionReach
	err := s.store.InTransaction(ctx, func(tx ports.DeletionTx) error {
		current, kind, roots, err := s.reach(ctx, tx, target, id)
		if err != nil {
			return err
		}
		if current.Token != token {
			return &ReachChangedError{Reach: current}
		}
		reach = current
		if len(roots) == 0 {
			return nil
		}
		return tx.Delete(ctx, kind, roots)
	})
	if err != nil {
		return nil, err
	}

	result := &domain.DeletionResult{Reach: *reach}
	// The deletion has committed; failing to compact does not undo it, so
	// it is reported rather than returned.
	if err := s.store.Compact(ctx, false); err != nil {
		slog.WarnContext(ctx, "deleted content may remain on disk until it is overwritten", "error", err)
		result.Notices = append(result.Notices,
			"The data was deleted, but the database file could not be compacted, so some of it may remain on disk until it is overwritten.")
	}
	return result, nil
}

func validateDeletion(target domain.DeletionTarget, id string) error {
	if !target.Valid() {
		return fmt.Errorf("%w: unknown target %q", ErrInvalidDeletion, target)
	}
	if target.TakesID() && id == "" {
		return fmt.Errorf("%w: deleting a %s needs its ID", ErrInvalidDeletion, target)
	}
	if !target.TakesID() && id != "" {
		return fmt.Errorf("%w: %s takes no ID", ErrInvalidDeletion, target)
	}
	return nil
}

// reach walks domain.DeletionRules from target's records and returns what
// the walk reached, the kind and IDs of the records deleted directly, and
// any refusal.
func (s *DeletionService) reach(ctx context.Context, tx ports.DeletionTx, target domain.DeletionTarget,
	id string) (*domain.DeletionReach, domain.RecordKind, []string, error) {
	kind, roots, err := tx.Roots(ctx, target, id)
	if err != nil {
		return nil, "", nil, err
	}
	if len(roots) == 0 && (target.TakesID() || target == domain.DeleteProfile) {
		return nil, "", nil, fmt.Errorf("%w: no %s %s", ErrNothingToDelete, target, id)
	}

	if target == domain.DeleteCoverLetter {
		current, err := tx.IsCurrentCoverLetter(ctx, id)
		if err != nil {
			return nil, "", nil, err
		}
		if current {
			// The letter an application uses must not change without the
			// user choosing another first.
			return nil, "", nil, fmt.Errorf(
				"%w: it is the cover letter its application uses — choose another version first, or delete the application",
				ErrDeletionRefused)
		}
	}

	deleted := map[domain.RecordKind]map[string]bool{kind: idSet(roots)}
	detached := map[domain.RecordKind]map[string]bool{}

	type frontier struct {
		kind domain.RecordKind
		ids  []string
	}
	queue := []frontier{{kind, roots}}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, rule := range domain.RulesReferencing(next.kind) {
			referencing, err := tx.Referencing(ctx, rule, next.ids)
			if err != nil {
				return nil, "", nil, err
			}
			if rule.Effect == domain.DeletionDetach {
				addIDs(detached, rule.Referencing, referencing)
				continue
			}
			var fresh []string
			for _, ref := range referencing {
				if !deleted[rule.Referencing][ref] {
					fresh = append(fresh, ref)
				}
			}
			if len(fresh) > 0 {
				addIDs(deleted, rule.Referencing, fresh)
				queue = append(queue, frontier{rule.Referencing, fresh})
			}
		}
	}
	// A record that goes is not also one that stays detached.
	for k, ids := range detached {
		for id := range ids {
			if deleted[k][id] {
				delete(ids, id)
			}
		}
	}

	reach := &domain.DeletionReach{
		Target:   target,
		ID:       id,
		Deleted:  countByKind(deleted),
		Detached: countByKind(detached),
	}
	if reach.SentApplications, err = tx.SentApplications(ctx, sortedIDs(deleted[domain.RecordApplication])); err != nil {
		return nil, "", nil, err
	}
	if target == domain.ClearJobs {
		if reach.KeptNotInterested, err = tx.KeptNotInterested(ctx); err != nil {
			return nil, "", nil, err
		}
	}
	reach.Token = reachToken(target, id, deleted, detached)
	return reach, kind, roots, nil
}

// everythingReach is the reach of "delete all data": every record, and
// what is kept outside the database.
func (s *DeletionService) everythingReach(ctx context.Context) (*domain.DeletionReach, error) {
	reach := &domain.DeletionReach{Target: domain.DeleteEverything, Detached: map[domain.RecordKind]int{}}
	var fingerprint string
	err := s.store.InTransaction(ctx, func(tx ports.DeletionTx) error {
		var err error
		if reach.Deleted, err = tx.CountAll(ctx); err != nil {
			return err
		}
		fingerprint, err = tx.Fingerprint(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}

	if s.browserProfile != nil {
		reach.Outside = append(reach.Outside, s.browserProfile.Describe())
	}
	refs, err := s.credentials.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	if len(refs) > 0 {
		reach.Outside = append(reach.Outside, fmt.Sprintf("%d API %s in the system keychain", len(refs), plural(len(refs), "key", "keys")))
	}
	for _, ref := range refs {
		// Only a ref JobPilot recorded can be found: environment variables
		// are not enumerated, since reading every one to find ours would
		// be reading the user's whole environment.
		if s.credentials.FromEnvironment(ref) {
			reach.EnvironmentCredentials = append(reach.EnvironmentCredentials, ref.EnvVarName())
		}
	}
	if s.log != nil {
		reach.Outside = append(reach.Outside, s.log.Describe())
	}

	lines := []string{"everything", fingerprint}
	for _, ref := range refs {
		lines = append(lines, "credential "+ref.String())
	}
	reach.Token = hashLines(lines)
	return reach, nil
}

// deleteEverything runs "delete all data" in the order ADR-0021 §7 sets,
// which makes a failed run safe to repeat: every step is a no-op on what an
// earlier run already removed.
func (s *DeletionService) deleteEverything(ctx context.Context, token string) (*domain.DeletionResult, error) {
	reach, err := s.everythingReach(ctx)
	if err != nil {
		return nil, err
	}
	if reach.Token != token {
		return nil, &ReachChangedError{Reach: reach}
	}
	result := &domain.DeletionResult{Reach: *reach}

	if s.browserProfile != nil {
		if err := s.browserProfile.Erase(ctx); err != nil {
			return nil, fmt.Errorf("delete all data stopped at the browser profile, and nothing else was deleted: %w", err)
		}
	}

	refs, err := s.credentials.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("delete all data stopped at the API keys: %w", err)
	}
	for _, ref := range refs {
		err := s.credentials.Delete(ctx, ref.Source, ref.Label)
		switch {
		case errors.Is(err, ports.ErrCredentialFromEnvironment):
			result.Notices = append(result.Notices, fmt.Sprintf(
				"The environment variable %s still supplies an API key. JobPilot cannot unset it; remove it where it is set.", ref.EnvVarName()))
		case err != nil:
			return nil, fmt.Errorf("delete all data stopped at the API key for %s, after removing the browser profile: %w", ref, err)
		}
	}

	if err := s.store.InTransaction(ctx, func(tx ports.DeletionTx) error { return tx.DeleteAll(ctx) }); err != nil {
		return nil, fmt.Errorf("delete all data stopped at the database, after removing the browser profile and API keys: %w", err)
	}
	if err := s.store.Compact(ctx, true); err != nil {
		slog.WarnContext(ctx, "deleted content may remain on disk until it is overwritten", "error", err)
		result.Notices = append(result.Notices,
			"The data was deleted, but the database file could not be compacted, so some of it may remain on disk until it is overwritten.")
	}

	if s.log != nil {
		if err := s.log.Erase(ctx); err != nil {
			return nil, fmt.Errorf("delete all data deleted everything but the log: %w", err)
		}
	}
	return result, nil
}

// reachToken identifies exactly the records a reach covers.
func reachToken(target domain.DeletionTarget, id string, deleted, detached map[domain.RecordKind]map[string]bool) string {
	var lines []string
	for kind, ids := range deleted {
		for id := range ids {
			lines = append(lines, "deleted "+string(kind)+" "+id)
		}
	}
	for kind, ids := range detached {
		for id := range ids {
			lines = append(lines, "detached "+string(kind)+" "+id)
		}
	}
	sort.Strings(lines)
	return hashLines(append([]string{string(target) + " " + id}, lines...))
}

// hashLines is a token over lines, in order.
func hashLines(lines []string) string {
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
}

func idSet(ids []string) map[string]bool {
	s := make(map[string]bool, len(ids))
	for _, id := range ids {
		s[id] = true
	}
	return s
}

func addIDs(into map[domain.RecordKind]map[string]bool, kind domain.RecordKind, ids []string) {
	if len(ids) == 0 {
		return
	}
	if into[kind] == nil {
		into[kind] = map[string]bool{}
	}
	for _, id := range ids {
		into[kind][id] = true
	}
}

func sortedIDs(s map[string]bool) []string {
	ids := make([]string, 0, len(s))
	for id := range s {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func countByKind(sets map[domain.RecordKind]map[string]bool) map[domain.RecordKind]int {
	c := map[domain.RecordKind]int{}
	for kind, ids := range sets {
		if len(ids) > 0 {
			c[kind] = len(ids)
		}
	}
	return c
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
