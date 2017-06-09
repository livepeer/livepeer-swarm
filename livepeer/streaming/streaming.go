package streaming

import "context"

//Broadcaster takes a streamID and a reader, and broadcasts the data to whatever underlining network.
//Example:
// s := GetStream("StrmID")
// b := ppspp.NewBroadcaster("StrmID", s.Metadata())
// for seqNo, data := range s.Segments() {
// 	b.Broadcast(seqNo, data)
// }
type Broadcaster interface {
	Broadcast(seqNo uint64, data []byte) error
}

//Subscriber subscribes to a stream defined by strmID.  It returns a reader that contains the stream.
//Example:
//	sub, metadata := ppspp.NewSubscriber("StrmID")
//	stream := NewStream("StrmID", metadata)
//	err := sub.Subscribe(context.Background(), func(seqNo uint64, data []byte){
//		stream.WriteSeg(seqNo, data)
//	})
type Subscriber interface {
	Subscribe(ctx context.Context, f func(seqNo uint64, data []byte)) error
}
