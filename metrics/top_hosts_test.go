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

func TestHostCountHeapPushPanicsForNonHostCount(t *testing.T) {
	var heap HostCountHeap

	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("expected Push to panic for non-HostCount value")
		}
	}()

	heap.Push("not a host count")
}

func hostCountsByName(counts []HostCount) map[string]int {
	result := make(map[string]int, len(counts))
	for _, count := range counts {
		result[count.Host] = count.Count
	}
	return result
}
