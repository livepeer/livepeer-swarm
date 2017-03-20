package lpms

import (
	"context"
	"testing"

	"github.com/livepeer/go-livepeer/lpms/io"
)

type TestStream struct{}

type TestStreamDB struct{}

func (s *TestStreamDB) findStreamByID(id string) (io.Stream, error) {
	return nil, nil
}

type Location struct {
	StreamID   string
	StreamPath string
}

func getStreamID(path string) string {
	return "test"
}

func TestRunServer(t *testing.T) {
	streamDB := &TestStreamDB{}
	server, _ := NewVideoServer(&Config{RtmpPort: "1935"})

	//Playing videos from the source
	server.HandleRtmpPlay(func(ctx context.Context, reqPath string, dst io.Stream) error {
		streamID := getStreamID(reqPath)
		stream, _ := streamDB.findStreamByID(streamID)
		dst.WriteStream(ctx, stream)
		return nil
	})

	//Play HLS video - segment by segment.
	server.HandleHlsPlay(func(ctx context.Context, reqPath string, dst io.Stream) error {
		streamID := getStreamID(reqPath)
		stream, _ := streamDB.findStreamByID(streamID)
		dst.WriteStream(ctx, stream)
		return nil
	})

	// //Publishing into the source
	// server.HandlePublish(func(ctx context.Context, src io.Stream) (streamID string) {
	// 	stream := streamDB.NewStream()
	// 	io.CopyStream(ctx, src, stream)
	// 	return stream.ID
	// })

	// //Transcoding
	// streamID := "testStreamID"
	// inStream := streamDB.findStreamByStreamID(streamID)
	// // outBitRates := ["150", "350", "550"]
	// // outStreams := []io.Stream
	// // for rate := range outRates {
	// // 	outStreams.append(streamDB.CreateStream())
	// // }
	// transcodeConfig := &TranscodeConfig{
	// 	inCodec: Codec.H264,
	// 	outCodecs: Codec.H264,
	// 	inRate: inStream.bitRate(),
	// 	outRates: outBitrates,
	// 	inFormat: Format.RTMP,
	// 	outFormat: Format.HLS
	// }
	// // server.HandleTranscode(transcodeConfig, func(ctx context.Context, tranInput io.Stream, tranOutputs []io.Stream) {

	// //This is if transcode req is coming in from the local http req - simply forward it to the Swarm.  It'll figure out where to send it.
	// server.HandleTranscode(func(ctx context.Context, config *TranscodeConfig) {
	// 	self.Swarm.SendTranscodeReq(config)
	// })

	// //Upon recieving a transcode req and determining you are the transcoding node -
	// server.StartTranscode(ctx context.Context, transcodeConfig, func(tranInput io.Stream, tranOutputs []io.Stream) {
	// 	io.CopyStream(ctx, inStream, tranInput)
	// 	tranOutput, i := range tranOutputs {
	// 		outStream := streamDB.CreateStream()
	// 		outStreams.append(outStream)
	// 		io.CopyStream(ctx, tranOutput, outStream)
	// 	}
	// })

	// //Get all streams
	// streams := server.GetStreams() //[]io.Stream

}

// func TestHandleStreamLookup(t *testing.T) {
// 	server, nil := NewVideoServer(nil, nil)
// 	server.HandleStreamLookup(func(streamID string) (io.Stream, error) {

// 	})
// 	// server.HandlePlay(context.Background(), func(streamID string) (io.Stream, error) {
// 	// 	//Look up by streamID
// 	// 	s := &TestStream{}

// 	// 	return s, nil
// 	// })
// }

// func TestHandleStream(t *testing.T) {

// }
