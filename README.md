# What is Ratnet?
Ratnet is a library that allows applications to communicate using an *onion-routed* and *flood-routed* message bus.  All communications are encrypted end-to-end by the library itself.

Ratnet is completely modular, meaning that the interactions of all significant components of the system are defined with interfaces, and are therefore interchangeable.  Network transports, cryptosystems, connection policies, and the ratnet nodes themselves can all be customized and swapped around dynamically.

The Ratnet library provides two working implementations for each of these interfaces:

- Network Transports:  [HTTPS](https://godoc.org/github.com/awgh/ratnet/transports/https) and [UDP](https://godoc.org/github.com/awgh/ratnet/transports/udp) are provided
- Cryptosystems: [ECC](https://godoc.org/github.com/awgh/bencrypt/ecc) and [RSA](https://godoc.org/github.com/awgh/bencrypt/ecc) implementations are provided
- Connection Policies: [Server](https://godoc.org/github.com/awgh/ratnet/policy#Server) and [Polling](https://godoc.org/github.com/awgh/ratnet/policy#Poll) are provided
- Nodes: [QL Database Backed Node](https://godoc.org/github.com/awgh/ratnet/nodes/qldb) and a [RAM-only Node](https://godoc.org/github.com/awgh/ratnet/nodes/ram) are provided

It's also easy to implement your own replacement for any or all of these components.  Multiple transport modules can be used at once, and different cryptosystems can be used for the Onion-routing and for the content encryption, if desired.

Ratnet provides input and output channels for your application to send and receive binary messages in clear text, making it very easy to interact with.

# Examples

## Making a Node

Make a QL-Database-Backed Node, which saves states to disk:
```go
	// QLDB Node Mode
	node := qldb.New(new(ecc.KeyPair), new(ecc.KeyPair))
	node.BootstrapDB(dbFile)
```

Or, make a RAM-Only Node, which won't write anything to the disk:
```go
	// RamNode Mode:
	node := ram.New(new(ecc.KeyPair), new(ecc.KeyPair))
```

The KeyPairs passed in as arguments are just used to determine which cryptosystem should be used for the Onion-Routing (first argument) and for the Content Encryption (second argument).  

## Setup Transports and Policies 

```go
	transportPublic := https.New("cert.pem", "key.pem", node, true)
	transportAdmin := https.New("cert.pem", "key.pem", node, true)
	node.SetPolicy(
		policy.NewServer(transportPublic, listenPublic, false),
		policy.NewServer(transportAdmin, listenAdmin, true))

	log.Println("Public Server starting: ", listenPublic)
	log.Println("Control Server starting: ", listenAdmin)

	node.Start()
```	

## Handle messages coming from the network

```go	
	go func() {
		for {
			msg := <-node.Out()
			if err := HandleMsg(msg); err != nil {
				log.Println(err.Error())
			}
		}
	}()
```

## Send messages to the network

Blocking Send:
```go
	message := api.Msg{Name: "destname1", IsChan: false}
	message.Content = bytes.NewBufferString(testMessage1)
	node.In() <- message
```
	
Non-Blocking Send:
```go
        select {
		case node.Out() <- message:
			//fmt.Println("sent message", msg)
		default:
			//fmt.Println("no message sent")
		}	
```

# Additional Documentation

- Overview Slide Deck from Toorcamp 2016 [here](https://github.com/awgh/ratnet/blob/master/docs/RatNet-Toorcamp16-v1.pdf).
- API Docs are available [here](https://godoc.org/github.com/awgh/ratnet/api).

# Related Projects

- [Bencrypt, crypto abstraction layer & utils](https://github.com/awgh/bencrypt)
- [Ratnet, onion-routed messaging system with pluggable transports](https://github.com/awgh/ratnet)
- [HushCom, simple IRC-like client & server](https://github.com/awgh/hushcom)

#Authors and Contributors

- awgh@awgh.org (@awgh)

- vyrus001@gmail.com (@vyrus001)
