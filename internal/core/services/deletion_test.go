package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/unicdev/jobpilot/internal/core/domain"
	"github.com/unicdev/jobpilot/internal/core/ports"
)

// fakeRecord is one stored record and the records it references, by kind.
type fakeRecord struct {
	kind          domain.RecordKind
	refs          map[domain.RecordKind]string
	sent          bool
	notInterested bool
}

// fakeDeletionStore models stored data as records and references, so
// DeletionService's walk over domain.DeletionRules runs against the real
// rules. It records what it is asked to delete rather than cascading:
// cascading is the schema's job, tested in the SQLite adapter.
type fakeDeletionStore struct {
	records map[string]*fakeRecord
	// betweenPreviewAndDelete runs once, at the start of the first
	// transaction after it is set: the moment a scout could act.
	betweenPreviewAndDelete func(*fakeDeletionStore)
	calls                   *[]string
	compactErr              error
}

func (f *fakeDeletionStore) add(id string, kind domain.RecordKind, refs map[domain.RecordKind]string) *fakeRecord {
	r := &fakeRecord{kind: kind, refs: refs}
	f.records[id] = r
	return r
}

func (f *fakeDeletionStore) InTransaction(_ context.Context, do func(ports.DeletionTx) error) error {
	if hook := f.betweenPreviewAndDelete; hook != nil {
		f.betweenPreviewAndDelete = nil
		hook(f)
	}
	return do(f)
}

func (f *fakeDeletionStore) Compact(_ context.Context, full bool) error {
	*f.calls = append(*f.calls, fmt.Sprintf("compact full=%v", full))
	return f.compactErr
}

func (f *fakeDeletionStore) idsOf(kind domain.RecordKind, keep func(*fakeRecord) bool) []string {
	var ids []string
	for id, r := range f.records {
		if r.kind == kind && (keep == nil || keep(r)) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (f *fakeDeletionStore) hasApplication(jobID string) bool {
	for _, r := range f.records {
		if r.kind == domain.RecordApplication && r.refs[domain.RecordJob] == jobID {
			return true
		}
	}
	return false
}

func (f *fakeDeletionStore) Roots(_ context.Context, target domain.DeletionTarget, id string) (domain.RecordKind, []string, error) {
	single := map[domain.DeletionTarget]domain.RecordKind{
		domain.DeleteJob: domain.RecordJob, domain.DeleteApplication: domain.RecordApplication,
		domain.DeleteCoverLetter: domain.RecordCoverLetter, domain.DeleteResume: domain.RecordResume,
	}
	if kind, ok := single[target]; ok {
		if r, found := f.records[id]; found && r.kind == kind {
			return kind, []string{id}, nil
		}
		return kind, nil, nil
	}
	switch target {
	case domain.DeleteProfile:
		return domain.RecordCandidate, f.idsOf(domain.RecordCandidate, nil), nil
	case domain.ClearActivity:
		return domain.RecordAgentRun, f.idsOf(domain.RecordAgentRun, nil), nil
	case domain.ClearJobs:
		return domain.RecordJob, f.clearableJobs(), nil
	}
	return "", nil, fmt.Errorf("no roots for %s", target)
}

// clearableJobs are the jobs "clear jobs" deletes: none an application
// uses, none marked not interested.
func (f *fakeDeletionStore) clearableJobs() []string {
	var ids []string
	for _, id := range f.idsOf(domain.RecordJob, func(r *fakeRecord) bool { return !r.notInterested }) {
		if !f.hasApplication(id) {
			ids = append(ids, id)
		}
	}
	return ids
}

func (f *fakeDeletionStore) Referencing(_ context.Context, rule domain.DeletionRule, ids []string) ([]string, error) {
	wanted := idSet(ids)
	return f.idsOf(rule.Referencing, func(r *fakeRecord) bool { return wanted[r.refs[rule.Referenced]] }), nil
}

func (f *fakeDeletionStore) SentApplications(_ context.Context, ids []string) (int, error) {
	n := 0
	for _, id := range ids {
		if f.records[id].sent {
			n++
		}
	}
	return n, nil
}

func (f *fakeDeletionStore) KeptNotInterested(context.Context) (int, error) {
	return len(f.idsOf(domain.RecordJob, func(r *fakeRecord) bool { return r.notInterested })), nil
}

func (f *fakeDeletionStore) IsCurrentCoverLetter(_ context.Context, id string) (bool, error) {
	for _, r := range f.records {
		if r.kind == domain.RecordApplication && r.refs[domain.RecordCoverLetter] == id {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeDeletionStore) Delete(_ context.Context, kind domain.RecordKind, ids []string) error {
	*f.calls = append(*f.calls, fmt.Sprintf("delete %s %s", kind, strings.Join(ids, ",")))
	return nil
}

func (f *fakeDeletionStore) CountAll(context.Context) (map[domain.RecordKind]int, error) {
	c := map[domain.RecordKind]int{}
	for _, r := range f.records {
		c[r.kind]++
	}
	return c, nil
}

func (f *fakeDeletionStore) Fingerprint(context.Context) (string, error) {
	ids := make([]string, 0, len(f.records))
	for id := range f.records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return strings.Join(ids, ","), nil
}

func (f *fakeDeletionStore) DeleteAll(context.Context) error {
	*f.calls = append(*f.calls, "delete all records")
	f.records = map[string]*fakeRecord{}
	return nil
}

// newDeletionFixture stores one of everything a deletion can reach:
//
//	candidate c, resume r
//	job j1: match m1, application a1 (resume r, current letter cl1), with
//	        letters cl0 and cl1, question q1, transition t1
//	job j2: nothing uses it
//	job j3: marked not interested, nothing uses it
//	job j4: application a4, sent
//	scout sc: runs sr and sr2, and a step of each
func newDeletionFixture() (*fakeDeletionStore, *[]string) {
	calls := &[]string{}
	f := &fakeDeletionStore{records: map[string]*fakeRecord{}, calls: calls}
	R := func(pairs ...any) map[domain.RecordKind]string {
		m := map[domain.RecordKind]string{}
		for i := 0; i < len(pairs); i += 2 {
			m[pairs[i].(domain.RecordKind)] = pairs[i+1].(string)
		}
		return m
	}
	f.add("c", domain.RecordCandidate, nil)
	f.add("r", domain.RecordResume, R(domain.RecordCandidate, "c"))
	for _, j := range []string{"j1", "j2", "j3", "j4"} {
		f.add(j, domain.RecordJob, nil)
	}
	f.records["j3"].notInterested = true
	f.add("m1", domain.RecordMatch, R(domain.RecordJob, "j1", domain.RecordCandidate, "c"))
	f.add("a1", domain.RecordApplication, R(domain.RecordJob, "j1", domain.RecordCandidate, "c",
		domain.RecordResume, "r", domain.RecordCoverLetter, "cl1"))
	f.add("cl0", domain.RecordCoverLetter, R(domain.RecordApplication, "a1"))
	f.add("cl1", domain.RecordCoverLetter, R(domain.RecordApplication, "a1"))
	f.add("q1", domain.RecordQuestion, R(domain.RecordApplication, "a1"))
	f.add("t1", domain.RecordStateTransition, R(domain.RecordApplication, "a1"))
	f.add("a4", domain.RecordApplication, R(domain.RecordJob, "j4", domain.RecordCandidate, "c")).sent = true
	f.add("sc", domain.RecordScout, nil)
	f.add("sr", domain.RecordAgentRun, R(domain.RecordScout, "sc"))
	f.add("ss", domain.RecordAgentStep, R(domain.RecordAgentRun, "sr"))
	f.add("sr2", domain.RecordAgentRun, R(domain.RecordScout, "sc"))
	f.add("ss2", domain.RecordAgentStep, R(domain.RecordAgentRun, "sr2"))
	return f, calls
}

// fakeDeletionCredentials is the credential service as delete-all sees it.
type fakeDeletionCredentials struct {
	refs      []domain.CredentialRef
	fromEnv   map[string]bool
	deleteErr map[string]error
	calls     *[]string
}

func (f *fakeDeletionCredentials) List(context.Context) ([]domain.CredentialRef, error) {
	return f.refs, nil
}

func (f *fakeDeletionCredentials) Delete(_ context.Context, source, label string) error {
	ref := source + "/" + label
	*f.calls = append(*f.calls, "delete key "+ref)
	if err := f.deleteErr[ref]; err != nil {
		return err
	}
	if f.fromEnv[ref] {
		return fmt.Errorf("still in the environment: %w", ports.ErrCredentialFromEnvironment)
	}
	return nil
}

func (f *fakeDeletionCredentials) FromEnvironment(ref domain.CredentialRef) bool {
	return f.fromEnv[ref.String()]
}

type fakeEraser struct {
	name  string
	err   error
	calls *[]string
}

func (f *fakeEraser) Describe() string { return f.name }

func (f *fakeEraser) Erase(context.Context) error {
	*f.calls = append(*f.calls, "erase "+f.name)
	return f.err
}

type fakeDeletionAlarms struct {
	description string
	err         error
	calls       *[]string
}

func (f *fakeDeletionAlarms) DescribeAlarms(context.Context) string { return f.description }

func (f *fakeDeletionAlarms) RemoveAllAlarms(context.Context) error {
	*f.calls = append(*f.calls, "remove scout alarms")
	return f.err
}

func newDeletionService(store *fakeDeletionStore, calls *[]string) (*DeletionService, *fakeDeletionCredentials, *fakeEraser, *fakeEraser) {
	creds := &fakeDeletionCredentials{calls: calls, fromEnv: map[string]bool{}, deleteErr: map[string]error{}}
	browser := &fakeEraser{name: "the browser profile", calls: calls}
	log := &fakeEraser{name: "the daemon's log", calls: calls}
	return NewDeletionService(store, creds, browser, log), creds, browser, log
}

func TestDeletionService_Preview(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cases := []struct {
		name     string
		target   domain.DeletionTarget
		id       string
		deleted  map[domain.RecordKind]int
		detached map[domain.RecordKind]int
		sent     int
		kept     int
	}{
		{
			name: "a job takes its match, its applications and everything under them", target: domain.DeleteJob, id: "j1",
			deleted: map[domain.RecordKind]int{
				domain.RecordJob: 1, domain.RecordMatch: 1, domain.RecordApplication: 1, domain.RecordCoverLetter: 2,
				domain.RecordQuestion: 1, domain.RecordStateTransition: 1,
			},
			// The application's reference to its own letter does not count as
			// detached: the application goes too.
			detached: map[domain.RecordKind]int{},
		},
		{
			name: "a job whose application was sent says so", target: domain.DeleteJob, id: "j4",
			deleted:  map[domain.RecordKind]int{domain.RecordJob: 1, domain.RecordApplication: 1},
			detached: map[domain.RecordKind]int{}, sent: 1,
		},
		{
			name: "a resume leaves the applications that used it, detached", target: domain.DeleteResume, id: "r",
			deleted:  map[domain.RecordKind]int{domain.RecordResume: 1},
			detached: map[domain.RecordKind]int{domain.RecordApplication: 1},
		},
		{
			name: "a cover letter that is not current goes alone", target: domain.DeleteCoverLetter, id: "cl0",
			deleted:  map[domain.RecordKind]int{domain.RecordCoverLetter: 1},
			detached: map[domain.RecordKind]int{},
		},
		{
			name: "the profile takes everything about the candidate and nothing about the search", target: domain.DeleteProfile,
			deleted: map[domain.RecordKind]int{
				domain.RecordCandidate: 1, domain.RecordResume: 1, domain.RecordMatch: 1, domain.RecordApplication: 2,
				domain.RecordCoverLetter: 2, domain.RecordQuestion: 1, domain.RecordStateTransition: 1,
			},
			detached: map[domain.RecordKind]int{}, sent: 1,
		},
		{
			name: "clearing activity takes every run and its steps", target: domain.ClearActivity,
			deleted:  map[domain.RecordKind]int{domain.RecordAgentRun: 2, domain.RecordAgentStep: 2},
			detached: map[domain.RecordKind]int{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, calls := newDeletionFixture()
			svc, _, _, _ := newDeletionService(store, calls)

			reach, err := svc.Preview(ctx, tc.target, tc.id)

			require.NoError(t, err)
			assert.Equal(t, tc.deleted, reach.Deleted)
			assert.Equal(t, tc.detached, reach.Detached)
			assert.Equal(t, tc.sent, reach.SentApplications)
			assert.Equal(t, tc.kept, reach.KeptNotInterested)
			assert.NotEmpty(t, reach.Token)
			assert.Empty(t, *calls, "a preview deletes nothing")
		})
	}
}

func TestDeletionService_ClearJobs_KeepsJobsInUseAndRuledOut(t *testing.T) {
	t.Parallel()
	store, calls := newDeletionFixture()
	svc, _, _, _ := newDeletionService(store, calls)
	reach, err := svc.Preview(context.Background(), domain.ClearJobs, "")
	require.NoError(t, err)

	assert.Equal(t, map[domain.RecordKind]int{domain.RecordJob: 1}, reach.Deleted, "only j2: j1 and j4 have applications")
	assert.Equal(t, 1, reach.KeptNotInterested, "j3 is marked not interested, is kept, and the confirmation says so")

	_, err = svc.Delete(context.Background(), domain.ClearJobs, "", reach.Token)
	require.NoError(t, err)
	assert.Equal(t, "delete job j2", (*calls)[0])
}

func TestDeletionService_Refusals(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, calls := newDeletionFixture()
	svc, _, _, _ := newDeletionService(store, calls)

	_, err := svc.Preview(ctx, domain.DeleteCoverLetter, "cl1")
	require.ErrorIs(t, err, ErrDeletionRefused, "the letter an application uses")
	assert.Contains(t, err.Error(), "choose another version first")

	_, err = svc.Preview(ctx, domain.DeleteJob, "no-such-job")
	require.ErrorIs(t, err, ErrNothingToDelete)

	for _, bad := range []struct {
		target domain.DeletionTarget
		id     string
	}{{"planet", ""}, {domain.DeleteJob, ""}, {domain.ClearActivity, "j1"}} {
		_, err := svc.Preview(ctx, bad.target, bad.id)
		assert.ErrorIs(t, err, ErrInvalidDeletion, "%s %q", bad.target, bad.id)
	}

	_, err = svc.Delete(ctx, domain.DeleteJob, "j1", "")
	require.ErrorIs(t, err, ErrDeletionUnconfirmed, "a deletion is never carried out without its preview")
	assert.Empty(t, *calls)
}

func TestDeletionService_Delete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("with the preview's token, the target's row is deleted and the files compacted", func(t *testing.T) {
		t.Parallel()
		store, calls := newDeletionFixture()
		svc, _, _, _ := newDeletionService(store, calls)
		reach, err := svc.Preview(ctx, domain.DeleteJob, "j1")
		require.NoError(t, err)

		result, err := svc.Delete(ctx, domain.DeleteJob, "j1", reach.Token)

		require.NoError(t, err)
		assert.Equal(t, *reach, result.Reach)
		assert.Equal(t, []string{"delete job j1", "compact full=false"}, *calls)
		assert.Empty(t, result.Notices)
	})

	t.Run("a change since the preview refuses the deletion and shows what it reaches now", func(t *testing.T) {
		t.Parallel()
		store, calls := newDeletionFixture()
		svc, _, _, _ := newDeletionService(store, calls)
		reach, err := svc.Preview(ctx, domain.DeleteJob, "j1")
		require.NoError(t, err)

		// A scout creates a second application for the job meanwhile.
		store.betweenPreviewAndDelete = func(f *fakeDeletionStore) {
			f.add("a-new", domain.RecordApplication, map[domain.RecordKind]string{domain.RecordJob: "j1", domain.RecordCandidate: "c"})
		}
		_, err = svc.Delete(ctx, domain.DeleteJob, "j1", reach.Token)

		var changed *ReachChangedError
		require.ErrorAs(t, err, &changed)
		require.ErrorIs(t, err, ErrReachChanged)
		assert.Equal(t, 2, changed.Reach.Deleted[domain.RecordApplication])
		assert.NotEqual(t, reach.Token, changed.Reach.Token)
		assert.Empty(t, *calls, "nothing the user did not see is deleted")
	})

	t.Run("a deletion that cannot be compacted still happened, and says so", func(t *testing.T) {
		t.Parallel()
		store, calls := newDeletionFixture()
		store.compactErr = errors.New("database is locked")
		svc, _, _, _ := newDeletionService(store, calls)
		reach, err := svc.Preview(ctx, domain.DeleteResume, "r")
		require.NoError(t, err)

		result, err := svc.Delete(ctx, domain.DeleteResume, "r", reach.Token)

		require.NoError(t, err)
		require.Len(t, result.Notices, 1)
		assert.Contains(t, result.Notices[0], "may remain on disk")
	})
}

func TestDeletionService_DeleteEverything(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	adzuna, err := domain.NewCredentialRef("adzuna", "work")
	require.NoError(t, err)
	jooble, err := domain.NewCredentialRef("jooble", "default")
	require.NoError(t, err)

	t.Run("the preview names what goes outside the database", func(t *testing.T) {
		t.Parallel()
		store, calls := newDeletionFixture()
		svc, creds, _, _ := newDeletionService(store, calls)
		creds.refs = []domain.CredentialRef{adzuna, jooble}
		creds.fromEnv[jooble.String()] = true

		reach, err := svc.Preview(ctx, domain.DeleteEverything, "")

		require.NoError(t, err)
		assert.Equal(t, 4, reach.Deleted[domain.RecordJob])
		assert.Equal(t, []string{"the browser profile", "2 API keys in the system keychain", "the daemon's log"}, reach.Outside)
		assert.Equal(t, []string{jooble.EnvVarName()}, reach.EnvironmentCredentials)
	})

	t.Run("it runs in the order that is safe to repeat, and reports what it cannot do", func(t *testing.T) {
		t.Parallel()
		store, calls := newDeletionFixture()
		svc, creds, _, _ := newDeletionService(store, calls)
		creds.refs = []domain.CredentialRef{adzuna, jooble}
		creds.fromEnv[jooble.String()] = true
		reach, err := svc.Preview(ctx, domain.DeleteEverything, "")
		require.NoError(t, err)

		result, err := svc.Delete(ctx, domain.DeleteEverything, "", reach.Token)

		require.NoError(t, err)
		assert.Equal(t, []string{
			"erase the browser profile",
			"delete key adzuna/work",
			"delete key jooble/default",
			"delete all records",
			"compact full=true",
			"erase the daemon's log",
		}, *calls)
		require.Len(t, result.Notices, 1)
		assert.Contains(t, result.Notices[0], jooble.EnvVarName())
	})

	t.Run("a failed step stops it before the next", func(t *testing.T) {
		t.Parallel()
		store, calls := newDeletionFixture()
		svc, _, browser, _ := newDeletionService(store, calls)
		browser.err = errors.New("permission denied")
		reach, err := svc.Preview(ctx, domain.DeleteEverything, "")
		require.NoError(t, err)

		_, err = svc.Delete(ctx, domain.DeleteEverything, "", reach.Token)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "browser profile")
		assert.Equal(t, []string{"erase the browser profile"}, *calls)
	})

	t.Run("the scouts' schedules go first, so none is left firing for a scout that is gone", func(t *testing.T) {
		t.Parallel()
		store, calls := newDeletionFixture()
		svc, _, _, _ := newDeletionService(store, calls)
		alarms := &fakeDeletionAlarms{calls: calls, description: "2 scheduled scouts in the operating system's scheduler"}
		svc.WithScoutAlarms(alarms)
		reach, err := svc.Preview(ctx, domain.DeleteEverything, "")
		require.NoError(t, err)
		assert.Equal(t, "2 scheduled scouts in the operating system's scheduler", reach.Outside[0])

		_, err = svc.Delete(ctx, domain.DeleteEverything, "", reach.Token)

		require.NoError(t, err)
		assert.Equal(t, "remove scout alarms", (*calls)[0])
	})

	t.Run("schedules that cannot be removed stop it before anything is deleted", func(t *testing.T) {
		t.Parallel()
		store, calls := newDeletionFixture()
		svc, _, _, _ := newDeletionService(store, calls)
		svc.WithScoutAlarms(&fakeDeletionAlarms{calls: calls, err: errors.New("launchctl: permission denied")})
		reach, err := svc.Preview(ctx, domain.DeleteEverything, "")
		require.NoError(t, err)

		_, err = svc.Delete(ctx, domain.DeleteEverything, "", reach.Token)

		require.ErrorContains(t, err, "scheduled scouts")
		assert.Equal(t, []string{"remove scout alarms"}, *calls)
	})

	t.Run("anything stored since the preview refuses it", func(t *testing.T) {
		t.Parallel()
		store, calls := newDeletionFixture()
		svc, _, _, _ := newDeletionService(store, calls)
		reach, err := svc.Preview(ctx, domain.DeleteEverything, "")
		require.NoError(t, err)

		store.add("j5", domain.RecordJob, nil)
		_, err = svc.Delete(ctx, domain.DeleteEverything, "", reach.Token)

		require.ErrorIs(t, err, ErrReachChanged)
		assert.Empty(t, *calls)
	})
}
