package io

// func TestRTMPWithServer(t *testing.T) {
// server := &joy4rtmp.Server{Addr: ":1936"}
// 	var in *RTMPStream

// server.HandlePlay = func(demuxer *joy4rtmp.Conn) {
// 	dst := &RTMPStream{Output: conn}
// 	dst.Stream(context.Background(), in)
// }

// LPMS.HandlePublish = func(demux RTMPDemuxer) {
//
// in = &RTMPStream{Input: conn}
// }

// 	go server.ListenAndServe()

// 	ffmpegCmd := "ffmpeg"
// 	ffmpegArgs := []string{"-re", "-i", "../data/bunny2.mp4", "-c", "copy", "-f", "flv", "rtmp://localhost:1936/movie/stream"}
// 	go exec.Command(ffmpegCmd, ffmpegArgs...).Run()

// 	ffplayCmd := "ffplay"
// 	ffplayArgs := []string{"rtmp://localhost:1936/movie/stream"}
// 	go exec.Command(ffplayCmd, ffplayArgs...).Run()

// 	// ffmpeg -re -i movie.flv -c copy -f flv rtmp://localhost:1935/movie
// 	// ffplay rtmp://localhost:1935/movie

// 	time.Sleep(time.Second * 10)
// }

// func TestHLSWithServer(t *testing.T) {

// }

// func TestCreateRTMPStream(t *testing.T) {
// 	conn := &joy4rtmp.Conn{}

// 	dst := &RTMPStream{Output: conn}
// 	inStream := &RTMPStream{Input: conn}
// 	dst.Stream(context.Background(), inStream)
// 	dst.Close()

// }

// func TestRTMPStreamEOF(t *testing.T) {

// }

// func TestRTMPStreamPlayerClose(t *testing.T) {

// }

// func TestRTMPStream
