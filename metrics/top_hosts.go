package metrics

import (
	"container/heap"
	"sync"
)

type HostCount struct {
	Host  string
	Count int
}

type TopHosts struct {
	maxSize int
	counter map[string]int
	heap    HostCountHeap
	lock    sync.Mutex
}

func NewTopHosts(maxSize int) *TopHosts {
	return &TopHosts{
		maxSize: maxSize,
		counter: make(map[string]int),
		heap:    make(HostCountHeap, 0, maxSize),
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

type HostCountHeap []HostCount

func (h HostCountHeap) Len() int           { return len(h) }
func (h HostCountHeap) Less(i, j int) bool { return h[i].Count < h[j].Count }
func (h HostCountHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *HostCountHeap) Push(x interface{}) {
	*h = append(*h, x.(HostCount))
}

func (h *HostCountHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}
