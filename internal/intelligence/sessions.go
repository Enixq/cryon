package intelligence

import "context"

// SessionRepository keeps session updates and event insertion atomic. Duplicate
// event delivery must not increment the revision a second time.
type SessionRepository interface {
	CreateDJSession(context.Context, SessionProfile) error
	DJSession(context.Context, string) (SessionProfile, bool, error)
	RecordDJFeedback(context.Context, FeedbackEvent) (bool, error)
	DJFeedback(context.Context, string, int) ([]FeedbackEvent, error)
	EndDJSession(context.Context, string) error
	SaveDJCandidates(context.Context, string, string, []Candidate) error
	DJCandidates(context.Context, string, string) ([]Candidate, error)
}

func ValidFeedback(kind FeedbackKind) bool {
	switch kind {
	case FeedbackShown, FeedbackLike, FeedbackDislike, FeedbackSkip, FeedbackCompleted, FeedbackReplay, FeedbackPlaybackError:
		return true
	}
	return false
}
