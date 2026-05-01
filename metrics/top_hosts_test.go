package metrics

import "testing"

func TestTopHostsKeepsMostFrequentHosts(t *testing.T) {
	topHosts := NewTopHosts(2)

	topHosts.Add("one.example")
	topHosts.Add("two.example")
	topHosts.Add("two.example")
	topHosts.Add("three.example")
	topHosts.Add("three.example")
	topHosts.Add("three.example")

	got := hostCountsByName(topHosts.GetTop())

	if len(got) != 2 {
		t.Fatalf("expected exactly 2 top hosts, got %d: %#v", len(got), got)
	}
	if _, exists := got["one.example"]; exists {
		t.Fatalf("expected least frequent host to be evicted, got %#v", got)
	}
	if got["two.example"] != 2 {
		t.Fatalf("expected two.example count 2, got %d", got["two.example"])
	}
	if got["three.example"] != 3 {
		t.Fatalf("expected three.example count 3, got %d", got["three.example"])
	}
}

func TestTopHostsUpdatesExistingHostCount(t *testing.T) {
	topHosts := NewTopHosts(1)

	topHosts.Add("one.example")
	topHosts.Add("one.example")

	got := topHosts.GetTop()

	if len(got) != 1 {
		t.Fatalf("expected one host, got %d", len(got))
	}
	if got[0].Host != "one.example" {
		t.Fatalf("expected one.example, got %q", got[0].Host)
	}
	if got[0].Count != 2 {
		t.Fatalf("expected updated count 2, got %d", got[0].Count)
	}
}

func TestTopHostReasonsKeepsMostFrequentHostReasons(t *testing.T) {
	topHostReasons := NewTopHostReasons(2)

	topHostReasons.Add("one.example", "filtered_blocklist")
	topHostReasons.Add("two.example", "filtered_blocklist")
	topHostReasons.Add("two.example", "filtered_blocklist")
	topHostReasons.Add("three.example", "filtered_safe_search")
	topHostReasons.Add("three.example", "filtered_safe_search")
	topHostReasons.Add("three.example", "filtered_safe_search")

	got := hostReasonCountsByName(topHostReasons.GetTop())

	if len(got) != 2 {
		t.Fatalf("expected exactly 2 top host reasons, got %d: %#v", len(got), got)
	}
	if _, exists := got[HostReasonKey{Host: "one.example", Reason: "filtered_blocklist"}]; exists {
		t.Fatalf("expected least frequent host reason to be evicted, got %#v", got)
	}
	if got[HostReasonKey{Host: "two.example", Reason: "filtered_blocklist"}] != 2 {
		t.Fatalf("expected two.example filtered_blocklist count 2, got %d", got[HostReasonKey{Host: "two.example", Reason: "filtered_blocklist"}])
	}
	if got[HostReasonKey{Host: "three.example", Reason: "filtered_safe_search"}] != 3 {
		t.Fatalf("expected three.example filtered_safe_search count 3, got %d", got[HostReasonKey{Host: "three.example", Reason: "filtered_safe_search"}])
	}
}

func TestTopHostReasonsUpdatesExistingHostReasonCount(t *testing.T) {
	topHostReasons := NewTopHostReasons(1)

	topHostReasons.Add("one.example", "filtered_blocklist")
	topHostReasons.Add("one.example", "filtered_blocklist")

	got := topHostReasons.GetTop()

	if len(got) != 1 {
		t.Fatalf("expected one host reason, got %d", len(got))
	}
	if got[0].Host != "one.example" {
		t.Fatalf("expected one.example, got %q", got[0].Host)
	}
	if got[0].Reason != "filtered_blocklist" {
		t.Fatalf("expected filtered_blocklist, got %q", got[0].Reason)
	}
	if got[0].Count != 2 {
		t.Fatalf("expected updated count 2, got %d", got[0].Count)
	}
}

func TestHostCountHeapPushPanicsForNonHostCount(t *testing.T) {
	var heap HostCountHeap

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected Push to panic for non-HostCount value")
		}
	}()

	heap.Push("not a host count")
}

func TestHostReasonCountHeapPushPanicsForNonHostReasonCount(t *testing.T) {
	var heap HostReasonCountHeap

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected Push to panic for non-HostReasonCount value")
		}
	}()

	heap.Push("not a host reason count")
}

func hostCountsByName(counts []HostCount) map[string]int {
	result := make(map[string]int, len(counts))
	for _, count := range counts {
		result[count.Host] = count.Count
	}
	return result
}

func hostReasonCountsByName(counts []HostReasonCount) map[HostReasonKey]int {
	result := make(map[HostReasonKey]int, len(counts))
	for _, count := range counts {
		result[HostReasonKey{Host: count.Host, Reason: count.Reason}] = count.Count
	}
	return result
}
