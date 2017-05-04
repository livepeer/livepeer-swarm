package streaming

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/logger/glog"
	lpmsStream "github.com/livepeer/lpms/stream"
	"github.com/nareix/joy4/av"
	"github.com/nareix/joy4/av/pubsub"
	"github.com/nareix/joy4/codec/aacparser"
	"github.com/nareix/joy4/codec/h264parser"
)

// The ID for a stream, consists of the concatenation of the
// NodeID and a unique ID string of the
type StreamID string
type TranscodeID string

const HLSWaitTime = time.Second * 10

func RandomStreamID() common.Hash {
	rand.Seed(time.Now().UnixNano())
	var x common.Hash
	for i := 0; i < len(x); i++ {
		x[i] = byte(rand.Uint32())
	}
	return x
}

func MakeStreamID(nodeID common.Hash, id string) StreamID {
	return StreamID(fmt.Sprintf("%x%v", nodeID[:], id))
}

func (self *StreamID) String() string {
	return string(*self)
}

// Given a stream ID, return it's origin nodeID and the unique stream ID
func (self *StreamID) SplitComponents() (common.Hash, string) {
	strStreamID := string(*self)
	originComponentLength := common.HashLength * 2 // 32 bytes == 64 hexadecimal digits
	if len(strStreamID) != (originComponentLength * 2) {
		return common.Hash{}, ""
	}
	return common.HexToHash(strStreamID[:originComponentLength]), strStreamID[originComponentLength:]
}

type HlsSegment struct {
	Data []byte
	Name string
}

// The streamer brookers the video streams
type Streamer struct {
	// Streams        map[StreamID]*Stream
	networkStreams map[StreamID]*lpmsStream.VideoStream
	subscribers    map[StreamID]*lpmsStream.StreamSubscriber
	cancellation   map[StreamID]context.CancelFunc
	SelfAddress    common.Hash
}

func NewStreamer(selfAddress common.Hash) (*Streamer, error) {
	glog.Infof("Setting up new streamer with self address: %x", selfAddress[:])
	s := &Streamer{
		// Streams:        make(map[StreamID]*Stream),
		networkStreams: make(map[StreamID]*lpmsStream.VideoStream),
		subscribers:    make(map[StreamID]*lpmsStream.StreamSubscriber),
		cancellation:   make(map[StreamID]context.CancelFunc),
		SelfAddress:    selfAddress,
	}
	return s, nil
}

func (self *Streamer) GetRTMPBuffer(id string) (buf av.Demuxer) {
	// q := self.rtmpBuffers[StreamID(id)]
	mux := self.subscribers[StreamID(id)].GetRTMPBuffer(id)
	q, ok := mux.(*pubsub.Queue)
	if !ok {
		return nil
	}
	if q != nil {
		buf = q.Oldest()
	}
	return
}

//Subscribes to a RTMP stream.  This function should be called in combination with forwarder.stream(), or another mechanism that will
//populate the VideoStream associated with the id.
func (self *Streamer) SubscribeToRTMPStream(strmID string, subID string, mux av.Muxer) (err error) {
	strm := self.networkStreams[StreamID(strmID)]
	if strm == nil {
		//Create VideoStream
		strm = lpmsStream.NewVideoStream(strmID, lpmsStream.RTMP)
		self.networkStreams[StreamID(strmID)] = strm
	}

	sub := self.subscribers[StreamID(strmID)]
	if sub == nil {
		//Create Subscriber, start worker
		sub = lpmsStream.NewStreamSubscriber(strm)
		self.subscribers[StreamID(strmID)] = sub
		ctx, cancel := context.WithCancel(context.Background())
		go sub.StartRTMPWorker(ctx)
		self.cancellation[StreamID(strmID)] = cancel
	}

	go sub.SubscribeRTMP(subID, mux)

	return nil
}

func (self *Streamer) EndRTMPStream(strmID string) {
	strm := self.networkStreams[StreamID(strmID)]
	if strm != nil {
		strm.WriteRTMPTrailer()
	}
}

func (self *Streamer) GetHLSMuxer(strmID string, subID string) (mux lpmsStream.HLSMuxer) {
	sub := self.subscribers[StreamID(strmID)]
	if sub != nil {
		return sub.GetHLSMuxer(subID)
	}
	return nil
}

func (self *Streamer) SubscribeToHLSStream(strmID string, subID string, mux lpmsStream.HLSMuxer) error {
	strm := self.networkStreams[StreamID(strmID)]
	if strm == nil {
		strm = lpmsStream.NewVideoStream(strmID, lpmsStream.HLS)
		self.networkStreams[StreamID(strmID)] = strm
	}

	sub := self.subscribers[StreamID(strmID)]
	if sub == nil {
		sub = lpmsStream.NewStreamSubscriber(strm)
		self.subscribers[StreamID(strmID)] = sub
		ctx, cancel := context.WithCancel(context.Background())
		go sub.StartHLSWorker(ctx, HLSWaitTime)
		self.cancellation[StreamID(strmID)] = cancel
	}

	return sub.SubscribeHLS(subID, mux)
}

func (self *Streamer) UnsubscribeToHLSStream(strmID string, subID string) {
	sub := self.subscribers[StreamID(strmID)]
	if sub != nil {
		sub.UnsubscribeHLS(subID)
	} else {
		return
	}

	if !sub.HasSubscribers() {
		self.cancellation[StreamID(strmID)]() //Call cancel on hls worker
		delete(self.subscribers, StreamID(strmID))
		sid := StreamID(strmID)
		nID, _ := sid.SplitComponents()
		if self.SelfAddress != nID { //Only delete the networkStream if you are a relay node
			delete(self.networkStreams, StreamID(strmID))
		}
	}
}

func (self *Streamer) UnsubscribeToRTMPStream(strmID string, subID string) {
	sub := self.subscribers[StreamID(strmID)]
	if sub != nil {
		sub.UnsubscribeRTMP(subID)
	} else {
		return
	}

	if !sub.HasSubscribers() {
		self.cancellation[StreamID(strmID)]() //Call cancel on hls worker
		delete(self.subscribers, StreamID(strmID))
		delete(self.networkStreams, StreamID(strmID))
	}
}

func (self *Streamer) HasSubscribers(strmID string) bool {
	sub := self.subscribers[StreamID(strmID)]
	if sub != nil {
		return sub.HasSubscribers()
	}
	return false
}

func (self *Streamer) GetNetworkStream(id StreamID) *lpmsStream.VideoStream {
	return self.networkStreams[id]
}

func (self *Streamer) GetAllNetworkStreams() []*lpmsStream.VideoStream {
	streams := make([]*lpmsStream.VideoStream, 0, len(self.networkStreams))
	for _, s := range self.networkStreams {
		streams = append(streams, s)
	}
	return streams
}

func (self *Streamer) AddNewNetworkStream(format lpmsStream.VideoFormat) (strm *lpmsStream.VideoStream, err error) {
	uid := RandomStreamID()
	streamID := MakeStreamID(self.SelfAddress, fmt.Sprintf("%x", uid))

	strm = lpmsStream.NewVideoStream(streamID.String(), format)
	self.networkStreams[streamID] = strm

	// glog.V(logger.Info).Infof("Adding new video stream with ID: %v", streamID)
	return
}

func (self *Streamer) DeleteNetworkStream(streamID StreamID) {
	delete(self.networkStreams, streamID)
}

func (self *Streamer) CurrentStatus() string {
	var networkStreams []string
	for k := range self.networkStreams {
		networkStreams = append(networkStreams, k.String())
	}

	var subscribers []string
	for k, v := range self.subscribers {
		hls := v.HLSSubscribers()
		rtmp := v.RTMPSubscribers()
		subscribers = append(subscribers, fmt.Sprintf("\n%v:\n%v hls: %v\n%v rtmp: %v\n\n", k.String(), len(hls), hls, len(rtmp), rtmp))
	}

	return fmt.Sprintf("%v streams: %v\n\n%v subscribers: %v\n\n\n\n", len(networkStreams), networkStreams, len(subscribers), subscribers)
}

func VideoChunkToByteArr(chunk VideoChunk) []byte {
	var buf bytes.Buffer
	gob.Register(VideoChunk{})
	gob.Register(h264parser.CodecData{})
	gob.Register(aacparser.CodecData{})
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(chunk)
	if err != nil {
		fmt.Println("Error converting bytearr to chunk: ", err)
	}
	return buf.Bytes()
}

func ByteArrInVideoChunk(arr []byte) VideoChunk {
	var buf bytes.Buffer
	gob.Register(VideoChunk{})
	gob.Register(h264parser.CodecData{})
	gob.Register(aacparser.CodecData{})
	gob.Register(av.Packet{})

	buf.Write(arr)
	var chunk VideoChunk
	dec := gob.NewDecoder(&buf)
	err := dec.Decode(&chunk)
	if err != nil {
		fmt.Println("Error converting bytearr to chunk: ", err)
	}
	return chunk
}
