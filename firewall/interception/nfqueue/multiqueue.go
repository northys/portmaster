// +build linux

package nfqueue

// suspended for now

// import (
// 	"sync"
//
// 	"github.com/safing/portmaster/network/packet"
// )
//
// type multiQueue struct {
// 	qs []*NFQueue
// }
//
// func NewMultiQueue(min, max uint16) (mq *multiQueue) {
// 	mq = &multiQueue{make([]*NFQueue, 0, max-min)}
// 	for i := min; i < max; i++ {
// 		mq.qs = append(mq.qs, NewNFQueue(i))
// 	}
// 	return mq
// }
//
// func (mq *multiQueue) Process() <-chan packet.Packet {
// 	var (
// 		wg  sync.WaitGroup
// 		out = make(chan packet.Packet, len(mq.qs))
// 	)
// 	for _, q := range mq.qs {
// 		wg.Add(1)
// 		go func(ch <-chan packet.Packet) {
// 			for pkt := range ch {
// 				out <- pkt
// 			}
// 			wg.Done()
// 		}(q.Process())
// 	}
// 	go func() {
// 		wg.Wait()
// 		close(out)
// 	}()
// 	return out
// }
// func (mq *multiQueue) Destroy() {
// 	for _, q := range mq.qs {
// 		q.Destroy()
// 	}
// }
