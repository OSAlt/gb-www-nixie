package domain

import "time"

// SocialMetricState is one platform's stored follower/member count together
// with the moment it was last refreshed successfully.
//
// The age of RefreshedAt doubles as a heartbeat for the metrics worker: the
// value is rewritten on every successful update, so a platform whose
// RefreshedAt stops advancing is either failing to fetch or no longer being
// refreshed at all.
type SocialMetricState struct {
	Platform    string
	Followers   int
	RefreshedAt time.Time
}
