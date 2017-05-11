package network

import (
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/livepeer/go-livepeer/livepeer/streaming"
	lpmsStream "github.com/livepeer/lpms/stream"
	"github.com/nareix/joy4/av"
)

type peerMuxer struct {
	originNode common.Hash
	streamID   string
	peer       *peer
}

func (p *peerMuxer) WriteSegment(seqNo uint64, name string, duration float64, s []byte) error {
	// glog.Infof("Writing segment to peer %v", p.peer.Addr())
	t, _ := time.ParseDuration(fmt.Sprintf("%vs", duration))
	chunk := streaming.VideoChunk{
		ID:         streaming.DeliverStreamMsgID,
		Seq:        seqNo,
		HLSSegData: s,
		HLSSegName: name,
		Duration:   t,
	}

	msg := &streamRequestMsgData{
		OriginNode: p.originNode,
		StreamID:   p.streamID,
		SData:      streaming.VideoChunkToByteArr(chunk),
		Id:         streaming.DeliverStreamMsgID,
	}
	p.peer.stream(msg)
	return nil
}

func (p *peerMuxer) WriteHeader(header []av.CodecData) error {
	chunk := streaming.VideoChunk{
		ID:            streaming.DeliverStreamMsgID,
		Seq:           0,
		HeaderStreams: header,
	}

	msg := &streamRequestMsgData{
		OriginNode: p.originNode,
		StreamID:   p.streamID,
		SData:      streaming.VideoChunkToByteArr(chunk),
		Id:         streaming.DeliverStreamMsgID,
	}
	p.peer.stream(msg)
	return nil
}

func (p *peerMuxer) WritePacket(pkt av.Packet) error {
	chunk := streaming.VideoChunk{
		ID:     streaming.DeliverStreamMsgID,
		Packet: pkt,
	}

	msg := &streamRequestMsgData{
		OriginNode: p.originNode,
		StreamID:   p.streamID,
		SData:      streaming.VideoChunkToByteArr(chunk),
		Id:         streaming.DeliverStreamMsgID,
	}
	p.peer.stream(msg)
	return nil
}

func (p *peerMuxer) WriteTrailer() error {
	// glog.Infof("Writing trailer to peer")
	chunk := streaming.VideoChunk{
		ID: streaming.EOFStreamMsgID,
	}

	msg := &streamRequestMsgData{
		OriginNode: p.originNode,
		Format:     lpmsStream.RTMP,
		StreamID:   p.streamID,
		SData:      streaming.VideoChunkToByteArr(chunk),
		Id:         streaming.EOFStreamMsgID,
	}
	p.peer.stream(msg)
	return nil
}
