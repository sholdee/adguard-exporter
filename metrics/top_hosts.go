package metrics

import (
	"container/heap"
	"sync"
)

type HostCount struct {
	Host  string
	Count int
}

type HostReasonCount struct {
	Host   string
	Reason string
	Count  int
}

type HostReasonKey struct {
	Host   string
	Reason string
}

type TopHosts struct {
	maxSize int
	counter map[string]int
	heap    HostCountHeap
	lock    sync.Mutex
}

type TopHostReasons struct {
	maxSize int
	counter map[HostReasonKey]int
	heap    HostReasonCountHeap
	lock    sync.Mutex
}

func NewTopHosts(maxSize int) *TopHosts {
	return &TopHosts{
		maxSize: maxSize,
		counter: make(map[string]int),
		heap:    make(HostCountHeap, 0, maxSize),
	}
}

func NewTopHostReasons(maxSize int) *TopHostReasons {
	return &TopHostReasons{
		maxSize: maxSize,
		counter: make(map[HostReasonKey]int),
		heap:    make(HostReasonCountHeap, 0, maxSize),
	}
}

func (th *TopHosts) Add(host string) {
	th.lock.Lock()
	defer th.lock.Unlock()

	th.counter[host]++
	count := th.counter[host]

	for i, hc := range th.heap {
		if hc.Host == host {
			th.heap = append(th.heap[:i], th.heap[i+1:]...)
			heap.Init(&th.heap)
			break
		}
	}

	if len(th.heap) < th.maxSize {
		heap.Push(&th.heap, HostCount{Host: host, Count: count})
	} else if count > th.heap[0].Count {
		heap.Pop(&th.heap)
		heap.Push(&th.heap, HostCount{Host: host, Count: count})
	}
}

func (th *TopHosts) GetTop() []HostCount {
	th.lock.Lock()
	defer th.lock.Unlock()

	result := make([]HostCount, len(th.heap))
	copy(result, th.heap)
	return result
}

func (thr *TopHostReasons) Add(host, reason string) {
	thr.lock.Lock()
	defer thr.lock.Unlock()

	key := HostReasonKey{Host: host, Reason: reason}
	thr.counter[key]++
	count := thr.counter[key]

	for i, hc := range thr.heap {
		if hc.Host == host && hc.Reason == reason {
			thr.heap = append(thr.heap[:i], thr.heap[i+1:]...)
			heap.Init(&thr.heap)
			break
		}
	}

	if len(thr.heap) < thr.maxSize {
		heap.Push(&thr.heap, HostReasonCount{Host: host, Reason: reason, Count: count})
	} else if count > thr.heap[0].Count {
		heap.Pop(&thr.heap)
		heap.Push(&thr.heap, HostReasonCount{Host: host, Reason: reason, Count: count})
	}
}

func (thr *TopHostReasons) GetTop() []HostReasonCount {
	thr.lock.Lock()
	defer thr.lock.Unlock()

	result := make([]HostReasonCount, len(thr.heap))
	copy(result, thr.heap)
	return result
}

type HostCountHeap []HostCount

func (h HostCountHeap) Len() int           { return len(h) }
func (h HostCountHeap) Less(i, j int) bool { return h[i].Count < h[j].Count }
func (h HostCountHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *HostCountHeap) Push(x interface{}) {
	hostCount, ok := x.(HostCount)
	if !ok {
		panic("HostCountHeap.Push received non-HostCount value")
	}
	*h = append(*h, hostCount)
}

func (h *HostCountHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

type HostReasonCountHeap []HostReasonCount

func (h HostReasonCountHeap) Len() int           { return len(h) }
func (h HostReasonCountHeap) Less(i, j int) bool { return h[i].Count < h[j].Count }
func (h HostReasonCountHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *HostReasonCountHeap) Push(x interface{}) {
	hostReasonCount, ok := x.(HostReasonCount)
	if !ok {
		panic("HostReasonCountHeap.Push received non-HostReasonCount value")
	}
	*h = append(*h, hostReasonCount)
}

func (h *HostReasonCountHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
