package streaming

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/logger"
	"github.com/ethereum/go-ethereum/logger/glog"
	"github.com/kz26/m3u8"
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

// A stream represents one stream
type Stream struct {
	SrcVideoChan  chan *VideoChunk
	DstVideoChan  chan *VideoChunk
	M3U8Chan      chan []byte
	HlsSegChan    chan HlsSegment
	HlsSegNameMap map[string][]byte
	M3U8          []byte
	lastDstSeq    int64
	ID            StreamID
	CloseChan     chan bool
}

func (self *Stream) PutToDstVideoChan(chunk *VideoChunk) {
	livepeerChunkInMeter.Mark(1)
	//Put to the stream
	if (chunk.HLSSegName != "") && (chunk.HLSSegData != nil) {
		//Should kick out old segments when the map reaches a certain size.
		self.HlsSegNameMap[chunk.HLSSegName] = chunk.HLSSegData
	} else if chunk.M3U8 != nil {
		self.M3U8 = chunk.M3U8
	} else {
		select {
		case self.DstVideoChan <- chunk:
			if self.lastDstSeq < chunk.Seq-1 {
				glog.V(logger.Info).Infof("Chunk skipped at %d\n", chunk.Seq)
				livepeerChunkSkipMeter.Mark(1)
			}
			self.lastDstSeq = chunk.Seq
		default:
		}
	}
}

func (self *Stream) PutToSrcVideoChan(chunk *VideoChunk) {
	select {
	case self.SrcVideoChan <- chunk:
	default:
	}
}

func (self *Stream) GetFromDstVideoChan() *VideoChunk {
	return <-self.DstVideoChan
}

func (self *Stream) GetFromSrcVideoChan() *VideoChunk {
	return <-self.SrcVideoChan
}

func (self *Stream) WriteHeader(streams []av.CodecData) error {
	glog.V(logger.Info).Infof("Write Header")
	chunk := &VideoChunk{
		ID:            DeliverStreamMsgID,
		Seq:           0,
		HeaderStreams: streams,
		Packet:        av.Packet{},
		M3U8:          nil,
		HLSSegData:    nil,
		HLSSegName:    "",
	}
	self.PutToSrcVideoChan(chunk)

	return nil
}

func (self *Stream) WritePacket(pkt av.Packet) error {
	chunk := &VideoChunk{
		ID:            DeliverStreamMsgID,
		Seq:           0,
		HeaderStreams: nil,
		Packet:        pkt,
		M3U8:          nil,
		HLSSegData:    nil,
		HLSSegName:    "",
	}
	self.PutToSrcVideoChan(chunk)
	return nil
}

func (self *Stream) WriteTrailer() error {
	glog.V(logger.Info).Infof("Write Trailer")
	chunk := &VideoChunk{
		ID:            EOFStreamMsgID,
		Seq:           0,
		HeaderStreams: nil,
		Packet:        av.Packet{},
		M3U8:          nil,
		HLSSegData:    nil,
		HLSSegName:    "",
	}
	self.PutToSrcVideoChan(chunk)

	return nil
}

func (self *Stream) Close() error {
	glog.V(logger.Info).Infof("Close")
	return nil
}

func (self *Stream) ReadPacket() (av.Packet, error) {
	glog.V(logger.Info).Infof("Read Packet")
	return av.Packet{}, errors.New("Not Implmented")
}

func (self *Stream) Streams() ([]av.CodecData, error) {
	glog.V(logger.Info).Infof("Streams")
	return nil, errors.New("Not Implmented")
}

func (self *Stream) WritePlaylist(pl m3u8.MediaPlaylist) error {
	chunk := &VideoChunk{
		ID:            DeliverStreamMsgID,
		Seq:           0,
		HeaderStreams: nil,
		Packet:        av.Packet{},
		M3U8:          pl.Encode().Bytes(),
		HLSSegData:    nil,
		HLSSegName:    "",
	}
	self.PutToSrcVideoChan(chunk)

	return nil
}

func (self *Stream) WriteSegment(name string, s []byte) error {
	chunk := &VideoChunk{
		ID:            DeliverStreamMsgID,
		Seq:           0,
		HeaderStreams: nil,
		Packet:        av.Packet{},
		M3U8:          nil,
		HLSSegData:    s,
		HLSSegName:    name,
	}
	self.PutToSrcVideoChan(chunk)
	return nil
}

// The streamer brookers the video streams
type Streamer struct {
	Streams        map[StreamID]*Stream
	networkStreams map[StreamID]*lpmsStream.VideoStream
	subscribers    map[StreamID]*lpmsStream.StreamSubscriber
	cancellation   map[StreamID]context.CancelFunc
	hlsBuffers     map[StreamID]lpmsStream.HLSMuxer
	rtmpBuffers    map[StreamID]*pubsub.Queue
	SelfAddress    common.Hash
}

func NewStreamer(selfAddress common.Hash) (*Streamer, error) {
	glog.Infof("Setting up new streamer with self address: %x", selfAddress[:])
	s := &Streamer{
		Streams:        make(map[StreamID]*Stream),
		networkStreams: make(map[StreamID]*lpmsStream.VideoStream),
		subscribers:    make(map[StreamID]*lpmsStream.StreamSubscriber),
		cancellation:   make(map[StreamID]context.CancelFunc),
		hlsBuffers:     make(map[StreamID]lpmsStream.HLSMuxer),
		rtmpBuffers:    make(map[StreamID]*pubsub.Queue),
		SelfAddress:    selfAddress,
	}
	return s, nil
}

func (self *Streamer) GetRTMPBuffer(id string) (buf av.Demuxer) {
	q := self.rtmpBuffers[StreamID(id)]
	if q != nil {
		buf = q.Oldest()
	}
	return
}

func (self *Streamer) GetAllRTMPBufferIDs() []StreamID {
	ids := make([]StreamID, 0, len(self.rtmpBuffers))
	for k, _ := range self.rtmpBuffers {
		ids = append(ids, k)
	}

	return ids
}

//Subscribes to a RTMP stream.  This function should be called in combination with forwarder.stream(), or another mechanism that will
//populate the VideoStream associated with the id.
func (self *Streamer) SubscribeToRTMPStream(ctx context.Context, strmID string, subID string, mux av.Muxer) (err error) {
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

	go sub.SubscribeRTMP(ctx, subID, mux)

	return nil
}

func (self *Streamer) EndRTMPStream(strmID string) {
	strm := self.networkStreams[StreamID(strmID)]
	if strm != nil {
		strm.WriteRTMPTrailer()
	}
}

func (self *Streamer) GetHLSMuxer(id string) (mux lpmsStream.HLSMuxer) {
	return self.hlsBuffers[StreamID(id)]
}

func (self *Streamer) GetAllHLSMuxerIDs() []StreamID {
	ids := make([]StreamID, 0, len(self.hlsBuffers))
	for k, _ := range self.hlsBuffers {
		ids = append(ids, k)
	}

	return ids
}

func (self *Streamer) SubscribeToHLSStream(ctx context.Context, strmID string, subID string, mux lpmsStream.HLSMuxer) error {
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
		go sub.StartHLSWorker(ctx)
		self.cancellation[StreamID(strmID)] = cancel
	}

	self.hlsBuffers[StreamID(strmID)] = mux
	return sub.SubscribeHLS(subID, mux)
}

func (self *Streamer) UnsubscribeToHLSStream(strmID string, subID string) {
	sub := self.subscribers[StreamID(strmID)]
	if sub != nil {
		sub.UnsubscribeHLS(subID)
	}

	if !sub.HasSubscribers() {
		self.cancellation[StreamID(strmID)]() //Call cancel on hls worker
		delete(self.subscribers, StreamID(strmID))
		delete(self.networkStreams, StreamID(strmID))
	}
}

func (self *Streamer) UnsubscribeToRTMPStream(strmID string, subID string) {
	sub := self.subscribers[StreamID(strmID)]
	if sub != nil {
		sub.UnsubscribeRTMP(subID)
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

// func (self *Streamer) SubscribeToHLSStream(ctx context.Context, strmID string, subID string) (buf *lpmsStream.HLSBuffer, err error) {
// 	buf = self.hlsBuffers[StreamID(strmID)]
// 	if buf != nil {
// 		return buf, nil
// 	}

// 	strm := self.networkStreams[StreamID(strmID)]
// 	if strm == nil {
// 		strm = lpmsStream.NewVideoStream(strmID)
// 		self.networkStreams[StreamID(strmID)] = strm
// 	}

// 	sub := self.subscribers[StreamID(strmID)]
// 	if sub == nil {
// 		sub = lpmsStream.NewStreamSubscriber(strm)
// 		self.subscribers[StreamID(strmID)] = sub
// 		go sub.StartHLSWorker(ctx)
// 	}

// 	buf = lpmsStream.NewHLSBuffer()
// 	sub.SubscribeHLS(subID, buf)

// 	// self.hlsBuffers[StreamID(id)] = buf
// 	// go strm.ReadHLSFromStream(ctx, buf)

// 	return
// }

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

func (self *Streamer) SubscribeToStream(id string) (stream *Stream, err error) {
	streamID := StreamID(id) //MakeStreamID(nodeID, id)
	glog.V(logger.Info).Infof("Subscribing to stream with ID: %v", streamID)
	return self.saveStreamForId(streamID)
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

func (self *Streamer) AddNewStream() (stream *Stream, err error) {
	//newID := // Generate random string for the stream
	uid := RandomStreamID()
	streamID := MakeStreamID(self.SelfAddress, fmt.Sprintf("%x", uid))
	glog.V(logger.Info).Infof("Adding new stream with ID: %v", streamID)
	return self.saveStreamForId(streamID)
}

func (self *Streamer) saveStreamForId(streamID StreamID) (stream *Stream, err error) {
	if self.Streams[streamID] != nil {
		return nil, errors.New("Stream with this ID already exists")
	}

	stream = &Stream{
		SrcVideoChan:  make(chan *VideoChunk, 10),
		DstVideoChan:  make(chan *VideoChunk, 10),
		M3U8Chan:      make(chan []byte),
		HlsSegChan:    make(chan HlsSegment),
		HlsSegNameMap: make(map[string][]byte),
		CloseChan:     make(chan bool),
		ID:            streamID,
	}
	self.Streams[streamID] = stream
	go func() {
		select {
		case <-stream.CloseChan:
			self.DeleteStream(stream.ID)
		}
	}()

	return stream, nil
}

func (self *Streamer) GetStream(nodeID common.Hash, id string) (stream *Stream, err error) {
	// TODO, return error if it doesn't exist
	return self.Streams[MakeStreamID(nodeID, id)], nil
}

func (self *Streamer) GetStreamByStreamID(streamID StreamID) (stream *Stream, err error) {
	return self.Streams[streamID], nil
}

func (self *Streamer) GetAllStreams() []StreamID {
	keys := make([]StreamID, 0, len(self.Streams))
	for k := range self.Streams {
		keys = append(keys, k)
	}
	return keys
}

func (self *Streamer) DeleteStream(streamID StreamID) {
	delete(self.Streams, streamID)
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

// func TestChunkEncoding(chunk VideoChunk) {
// 	bytes := VideoChunkToByteArr(chunk)
// 	newChunk := ByteArrInVideoChunk(bytes)
// 	fmt.Println("chunk: ", chunk)
// 	fmt.Println("newchunk: ", newChunk)
// }
