package syncer

import "errors"

const (
	syncStageAuthor  = "author"
	syncStageCommit  = "commit"
	syncStageFetch   = "fetch"
	syncStageCompare = "compare"
	syncStageRebase  = "rebase"
	syncStageAlert   = "alert"
	syncStagePush    = "push"
)

type syncError struct {
	stage string
	err   error
}

// @description    Creates a synchronization stage error.
//
// @param           stage  "name of the synchronization stage that failed"
//
// @param           err    "underlying stage error"
//
// @return          error  "error that preserves the stage and underlying cause"
func newSyncError(stage string, err error) error {
	return &syncError{stage: stage, err: err}
}

// @description    Returns the underlying synchronization error message.
//
// @return          string  "underlying error message"
func (e *syncError) Error() string {
	return e.err.Error()
}

// @description    Returns the underlying synchronization error.
//
// @return          error  "wrapped stage error"
func (e *syncError) Unwrap() error {
	return e.err
}

// @description    Returns the failed synchronization stage.
//
// @param           err     "error returned from AutoSync"
//
// @return          string  "stage name, or an empty string when the error has no stage information"
func SyncErrorStage(err error) string {
	var stageErr *syncError
	if !errors.As(err, &stageErr) {
		return ""
	}
	return stageErr.stage
}

// @description    Reports whether a synchronization error came from fetch or push.
//
// @param           err   "error returned from AutoSync"
//
// @return          bool  "true for fetch and push stage errors"
func IsRemoteSyncError(err error) bool {
	stage := SyncErrorStage(err)
	return stage == syncStageFetch || stage == syncStagePush
}
