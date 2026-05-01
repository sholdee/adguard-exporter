package metrics

import "time"

type FilteringReason int

const (
	ReasonNotFilteredNotFound FilteringReason = iota
	ReasonNotFilteredAllowList
	ReasonNotFilteredError
	ReasonFilteredBlockList
	ReasonFilteredSafeBrowsing
	ReasonFilteredParental
	ReasonFilteredInvalid
	ReasonFilteredSafeSearch
	ReasonFilteredBlockedService
	ReasonRewritten
	ReasonRewrittenAutoHosts
	ReasonRewrittenRule
)

type QueryLogResult struct {
	IsFiltered bool             `json:"IsFiltered"`
	Reason     *FilteringReason `json:"Reason"`
}

type QueryLogEntry struct {
	Time       time.Time `json:"T"`
	LegacyTime time.Time `json:"Time"`
	QHost      string    `json:"QH"`
	QType      string    `json:"QT"`
	QClass     string    `json:"QC"`
	Upstream   string
	Elapsed    *int64 `json:"Elapsed"`
	Cached     bool   `json:"Cached,omitempty"`
	Result     *QueryLogResult
}

func (e QueryLogEntry) Timestamp() time.Time {
	if !e.Time.IsZero() {
		return e.Time
	}
	return e.LegacyTime
}

func (e QueryLogEntry) SkipReason() string {
	switch {
	case e.Timestamp().IsZero():
		return "missing_timestamp"
	case e.QHost == "":
		return "missing_query_host"
	case e.QType == "":
		return "missing_query_type"
	case e.Elapsed == nil:
		return "missing_elapsed"
	case e.Result == nil:
		return "missing_result"
	case e.Result.IsFiltered && e.Result.Reason == nil:
		return "missing_reason"
	default:
		return ""
	}
}

func (e *QueryLogEntry) Normalize() {
	if e.Upstream == "" {
		e.Upstream = "unknown"
	}
	if e.Result != nil && !e.Result.IsFiltered && e.Result.Reason == nil {
		reason := ReasonNotFilteredNotFound
		e.Result.Reason = &reason
	}
}

func (e QueryLogEntry) ElapsedMilliseconds() float64 {
	if e.Elapsed == nil {
		return 0
	}
	return float64(*e.Elapsed) / float64(time.Millisecond)
}

func (r FilteringReason) Label() string {
	switch r {
	case ReasonNotFilteredNotFound:
		return "not_filtered_not_found"
	case ReasonNotFilteredAllowList:
		return "not_filtered_allow_list"
	case ReasonNotFilteredError:
		return "not_filtered_error"
	case ReasonFilteredBlockList:
		return "filtered_blocklist"
	case ReasonFilteredSafeBrowsing:
		return "filtered_safe_browsing"
	case ReasonFilteredParental:
		return "filtered_parental"
	case ReasonFilteredInvalid:
		return "filtered_invalid"
	case ReasonFilteredSafeSearch:
		return "filtered_safe_search"
	case ReasonFilteredBlockedService:
		return "filtered_blocked_service"
	case ReasonRewritten:
		return "rewritten"
	case ReasonRewrittenAutoHosts:
		return "rewritten_auto_hosts"
	case ReasonRewrittenRule:
		return "rewritten_rule"
	default:
		return "unknown"
	}
}

func (r FilteringReason) IsBlocked() bool {
	return r == ReasonFilteredBlockList || r == ReasonFilteredBlockedService
}

func (r FilteringReason) IsSafeSearch() bool {
	return r == ReasonFilteredSafeSearch
}
