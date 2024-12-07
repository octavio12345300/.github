package legacypool

import (
	"container/heap"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// txHeapItem implements the Interface interface of heap so that it can be heapified
type txHeapItem struct {
	tx        *types.Transaction
	timestamp int64 // Unix timestamp (nanoseconds) of when the transaction was added
	index     int
}

type txHeap []*txHeapItem

func (h txHeap) Len() int { return len(h) }
func (h txHeap) Less(i, j int) bool {
	return h[i].timestamp < h[j].timestamp
}
func (h txHeap) Swap(i, j int) {
	if i < 0 || j < 0 || i >= len(h) || j >= len(h) {
		return // Silently fail if indices are out of bounds
	}
	h[i], h[j] = h[j], h[i]
	if h[i] != nil {
		h[i].index = i
	}
	if h[j] != nil {
		h[j].index = j
	}
}

func (h *txHeap) Push(x interface{}) {
	item, ok := x.(*txHeapItem)
	if !ok {
		return
	}
	n := len(*h)
	item.index = n
	*h = append(*h, item)
}

func (h *txHeap) Pop() interface{} {
	old := *h
	n := len(old)
	if n == 0 {
		return nil // Return nil if the heap is empty
	}
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*h = old[0 : n-1]
	if item != nil {
		item.index = -1 // for safety
	}
	return item
}

type TxPool3Heap struct {
	txHeap    txHeap
	index     map[common.Hash]*txHeapItem
	mu        sync.RWMutex
	maxSize   uint64
	totalSize int
}

func NewTxPool3Heap(estimatedMaxSize uint64) *TxPool3Heap {
	return &TxPool3Heap{
		txHeap:  make(txHeap, 0, estimatedMaxSize),
		index:   make(map[common.Hash]*txHeapItem, estimatedMaxSize),
		maxSize: estimatedMaxSize,
	}
}

func (tp *TxPool3Heap) Add(tx *types.Transaction) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	if _, exists := tp.index[tx.Hash()]; exists {
		// Transaction already in pool, ignore
		return
	}

	if uint64(len(tp.txHeap)) >= tp.maxSize {
		// Remove the oldest transaction to make space
		oldestItem, ok := heap.Pop(&tp.txHeap).(*txHeapItem)
		if !ok || oldestItem == nil {
			return
		}
		delete(tp.index, oldestItem.tx.Hash())
		tp.totalSize -= numSlots(oldestItem.tx)
	}

	item := &txHeapItem{
		tx:        tx,
		timestamp: time.Now().UnixNano(),
	}
	heap.Push(&tp.txHeap, item)
	tp.index[tx.Hash()] = item
	tp.totalSize += numSlots(tx)
}

func (tp *TxPool3Heap) Get(hash common.Hash) (*types.Transaction, bool) {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	if item, ok := tp.index[hash]; ok {
		return item.tx, true
	}
	return nil, false
}

func (tp *TxPool3Heap) Remove(hash common.Hash) {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	if item, ok := tp.index[hash]; ok {
		heap.Remove(&tp.txHeap, item.index)
		delete(tp.index, hash)
		tp.totalSize -= numSlots(item.tx)
	}
}

func (tp *TxPool3Heap) Flush(n int) []*types.Transaction {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	if n > tp.txHeap.Len() {
		n = tp.txHeap.Len()
	}
	txs := make([]*types.Transaction, n)
	for i := 0; i < n; i++ {
		item, ok := heap.Pop(&tp.txHeap).(*txHeapItem)
		if !ok || item == nil {
			continue
		}
		txs[i] = item.tx
		delete(tp.index, item.tx.Hash())
		tp.totalSize -= numSlots(item.tx)
	}
	return txs
}

func (tp *TxPool3Heap) Len() int {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	return tp.txHeap.Len()
}

func (tp *TxPool3Heap) Size() int {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	return tp.totalSize
}

func (tp *TxPool3Heap) PrintTxStats() {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	for _, item := range tp.txHeap {
		tx := item.tx
		fmt.Printf("Hash: %s, Timestamp: %d, GasFeeCap: %s, GasTipCap: %s\n",
			tx.Hash().String(), item.timestamp, tx.GasFeeCap().String(), tx.GasTipCap().String())
	}
}
