## LivePeer

[Livepeer](https://livepeer.org) is a decentralized live streaming broadcast platform. This
repo is a proof-of-concept spike built on top of Swarm and 
[go-ethereum](https://github.com/ethereum/go-ethereum) developed by
the [Ethereum Foundation](http://ethereum.org).

Building and running this node allows you to:

* Create a local Livepeer Network, or join the existing Livepeer POC
network.
* Broadcast a live stream into the network.
* Request that your stream be transcoded into multiple formats.
* Consume a live stream from the network.

For full documentation and a project overview, check out the
[Livepeer Documentation](https://github.com/livepeer/wiki/wiki)

## Installation

The Livepeer POC requires ffmpeg. On OSX:

`brew install ffmpeg --with-ffplay`

or on Debian based Linux

`apt-get install ffmpeg`

If you have never set up your Go programming environment, do so according to [Go's Getting Started Guide](https://golang.org/doc/install).

Now fetch and build the `livepeer` node using go.

`go get github.com/livepeer/go-livepeer`  
`go install github.com/livepeer/go-livepeer/cmd/livepeer`

Or clone the repo and install the dependencies from source

`git clone https://github.com/livepeer/go-livepeer`  (do this within your $GOPATH under the proper directory structure)   
`cd go-livepeer`   
`go get ./...`  
`go install ./cmd/livepeer`

## Setup

If you would like to simply connect to the Livepeer Toynet (test
network with id=326326), **no setup is necessary.** Simply run the command:

`livepeer`

This will prompt you to create a new ethereum account, and unlock it
with a password.

If you would like to control where the data for Livepeer is stored,
and which account is used in each data directory (you may want to do
this if you're running multiple nodes), then use `geth` from
go-ethereum to create a new account in a
new directory:

(To build geth if you don't have it installed run `go install ./vendor/github.com/ethereum/go-ethereum/cmd/geth`)

`geth --datadir <datadir>  account new`

Copy the output account address, perhaps into an environment variable
$BZZKEY. You can then use this when you start livepeer

`livepeer --bzzaccount $BZZKEY --datadir <datadir>`

By default this should connect you to the Livepeer POC network. For
detailed instructions on all the options you can pass, refer to the following section.

### Detailed setup including running a private network

Since this spike runs on top of Swarm and an Ethereum node, you'll
need to do a little ethereum setup to get a node running and
initialize an account. Follow the instructions in the
[go-ethereum README](http://github.com/ethereum/go-ethereum) for
running a private network.

## Usage

The simplest way to start Livepeer and connect to the test network is
just by running:

`livepeer`

By default, starting Livepeer will launch an RTMP interface on
port 1935. You can override this with the --rtmp option:

`livepeer --rtmp 1937`

or more control over the account, data directory, and network you
connect to:

`livepeer --bzzaccount $BZZKEY --datadir $DATADIR --lpnetworkid 412 --rtmp 1935`

To run a second node, so that you can test streaming to one another,
specify a new `--port`, `--rtmp`, `--bzzport`, `--datadir`, and `--bzzaccount` argument:

`livepeer --bzzaccount $BZZKEY2 --datadir $DATADIR2  --port 30402 --lpnetworkid 412 --bzzport 8502 --rtmp 1936`

Now that you have two nodes running, make sure they are talking to
each other, then stream into one node and play from the other.  You
can stream a saved video using `ffmpeg`, For example:

`ffmpeg -re -i bunny.mp4 -c copy -f flv rtmp://localhost:1935/movie`

This will output the `streamID` to the console of node running on port
`1935` (the default). Copy the `streamID`. And you can play from the
second node (running on RTMP port 1936) using the livepeer command.

`livepeer stream --rtmp 1936 <streamID>`

To start a true livestream instead of playing a pre-recorded video, visit our web client or use a broadcasting
platform such as OBS, and point the output at `rtmp://localhost:1935/movie`

You can also use the web interface to test out streaming. To do that, make sure you are runing livepeer in the go-livepeer directory, and visit http://localhost:8935/. It should redirect you to http://localhost:8935/static/broadcast.html. Make sure the http port is your rtmp port +7000.


### Transcoding

Currently transcoding is an experimental feature.  Check out the [hls branch](https://github.com/livepeer/go-livepeer/tree/hls) for more informaiton.

## Metrics and monitoring

To look at a list of metrics, use the --metrics flag when starting
swarm, and use `geth monitor` to monitor metrics.  For example:

`livepeer --bzzaccount $BZZKEY --datadir $DATADIR --ethapi $DATADIR/geth.ipc --verbosity 4 --maxpeers 3 --port 30402 --lpnetworkid 412 --rtmp 1935 --metrics`

`geth monitor --attach ipc:/Users/erictang/Sandbox/swarmdata1/bzzd.ipc
livepeer/test livepeer/chunks/`

It is also possible to run a network visualization server which will
let you view the current state of your network for a given
streamID. See the documentation at the
[streamingviz repository](https://github.com/livepeer/streamingviz). To
start a livepeer node that reports to the visualization server, add
the `--viz` flag:

`livepeer --bzzaccount $BZZKEY --datadir $DATADIR --ethapi $DATADIR/geth.ipc --lpnetworkid 412 --rtmp 1935 --viz`

## Additional functionality available through Swarm and Ethereum

Since this spike is built on top of Swarm and Ethereum, all the
available functionality is exposed through this project. Please see
the documentation at [the go-ethereum repository](http://github.com/ethereum/go-ethereum)

### License information for go-livepeer and go-ethereum

This repo is largely based on a fork of the go-ethereum
codebase. The license and author information is included in `./vendor/github.com/ethereum/go-ethereum/`.

The go-livepeer library (i.e. all code outside of the `cmd` directory) is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html), also
included in our repository in the `COPYING.LESSER` file.

The go-livepeer binaries (i.e. all code inside of the `cmd` directory) is licensed under the
[GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also included
in our repository in the `COPYING` file.
