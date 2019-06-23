package server

import (
	"crypto/sha256"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"sync"
	"time"

	protoBlockchain "../../protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"github.com/syndtr/goleveldb/leveldb"
)

// ConnectionStore handles peer messaging
type ConnectionStore struct {
	clients    map[*client]bool
	broadcast  chan []byte
	register   chan *client
	unregister chan *client
}

type client struct {
	conn      *websocket.Conn
	send      chan []byte
	readOther chan []byte
	store     *ConnectionStore
	wg        sync.WaitGroup
	isOpen    bool
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Blockchain struct {
	BlockDb      *leveldb.DB
	CurrentBlock uint64
}

var chain *Blockchain

// StartServer start server
func StartServer(port string) {
	store := &ConnectionStore{
		clients:    make(map[*client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *client),
		unregister: make(chan *client),
	}

	go store.run()
	log.Info("Starting server on port ", port)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		// log.Info("New connection")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error(err)
			return
		}

		c := client{
			conn:      conn,
			send:      make(chan []byte, 256),
			readOther: make(chan []byte, 256),
			store:     store,
			isOpen:    true,
		}

		store.register <- &c

		go c.read()
		go c.write()
	})

	go http.ListenAndServe(":80", nil)

	// Server that allows peers to connect
	go http.ListenAndServe(port, nil)

	os.RemoveAll("blockchain")
	os.MkdirAll("blockchain", os.ModePerm)

	err := NewBlockchain("blockchain/blocks", 0)
	if err != nil {
		log.Error(err)
	}

	select {}
}

// run is the event handler to update the ConnectionStore
func (cs *ConnectionStore) run() {
	for {
		select {
		// A new client has registered
		case client := <-cs.register:
			client.wg.Add(1)
			client.wg.Done()
			cs.clients[client] = true

		// A client has quit, check if it exisited and delete it
		case client := <-cs.unregister:
			if _, ok := cs.clients[client]; ok {
				// Don't close the channel till we're done responding to avoid log.Errors
				client.wg.Wait()
				close(client.send)
				delete(cs.clients, client)
			}
		}
	}
}

// read reads data from the socket and handles it
func (c *client) read() {
	// Unregister if the node dies
	defer func() {
		// log.Info("Client died")
		// c.store.unregister <- c
		// c.isOpen = false
		c.conn.Close()
	}()

	for {
		msgType, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.BinaryMessage {
			continue
		}

		pb := protoBlockchain.Envelope{}
		err = proto.Unmarshal(msg, &pb)
		if err != nil {
			log.Error(err)
			continue
		}

		infoFile := protoBlockchain.File{}
		err = proto.Unmarshal(pb.GetData(), &infoFile)
		if err != nil {
			log.Error(err)
			continue
		}

		// create the hash of the file
		infos := []byte{}
		// infos = append(infos, infoFile.GetName()...)
		infos = append(infos, infoFile.GetSize()...)
		// infos = append(infos, infoFile.GetLastModification()...)
		infos = append(infos, infoFile.GetData()...)
		bhash := sha256.Sum256(infos)
		hash := bhash[:]

		go func() {
			switch pb.GetType() {

			case protoBlockchain.Envelope_UPLOAD:

				prevBlock, err := chain.GetBlock(chain.CurrentBlock)
				if err != nil {
					log.Error(err)
					break
				}
				bhash2 := sha256.Sum256(prevBlock)
				prevhash := bhash2[:]

				block := protoBlockchain.Block{
					Index:     chain.CurrentBlock + 1,
					Timestamp: uint64(time.Now().Unix()),
					PrevHash:  prevhash,
					HashFile:  hash,
				}

				chain.CurrentBlock++

				err = SaveBlock(&block)
				if err != nil {
					log.Error(err)
					break
				}

				env := &protoBlockchain.Envelope{
					Type: protoBlockchain.Envelope_UPLOAD,
					Data: hash,
				}
				data, err := proto.Marshal(env)
				if err != nil {
					log.Error(err)
					break
				}

				if c.isOpen {
					c.send <- data
				}

				log.Info("Succesfully uploaded")

			case protoBlockchain.Envelope_EXIST:
				var i uint64
				var find bool
				for i = 0; i <= chain.CurrentBlock; i++ {
					blockByte, _ := chain.GetBlock(i)
					block := &protoBlockchain.Block{}
					proto.Unmarshal(blockByte, block)

					hashCurrentFile := block.GetHashFile()
					log.Info(hashCurrentFile)
					timestamp := block.GetTimestamp()

					if reflect.DeepEqual(hashCurrentFile, hash) == true {
						env := &protoBlockchain.Envelope{
							Type: protoBlockchain.Envelope_EXIST,
							Data: []byte(strconv.Itoa(int(timestamp))),
						}
						data, err := proto.Marshal(env)
						if err != nil {
							log.Error(err)
							break
						}

						if c.isOpen {
							c.send <- data
						}

						find = true
						log.Info("Succesfully checked")
						break
					}
				}

				if !find {
					env := &protoBlockchain.Envelope{
						Type: protoBlockchain.Envelope_EXIST,
						Data: []byte{},
					}
					data, err := proto.Marshal(env)
					if err != nil {
						log.Error(err)
						break
					}

					if c.isOpen {
						c.send <- data
					}
					log.Info("Not succesfully checked")
				}
			}
		}()
	}
}

// NewBlockchain creates a database db
func NewBlockchain(dbPath string, index uint64) error {
	dbb, err := leveldb.OpenFile(dbPath+".blocks", nil)
	if err != nil {
		return err
	}

	chain = &Blockchain{
		BlockDb:      dbb,
		CurrentBlock: index,
	}

	block := &protoBlockchain.Block{
		Index:     index,
		Timestamp: uint64(time.Now().Unix()),
		HashFile:  []byte("GenesisBlock"),
	}

	err = SaveBlock(block)
	if err != nil {
		log.Error(err)
	}

	return err
}

// SaveBlock saves a block in the chain
func SaveBlock(block *protoBlockchain.Block) error {
	res, _ := proto.Marshal(block)
	return chain.BlockDb.Put([]byte(strconv.Itoa(int(block.GetIndex()))), res, nil)
}

// GetBlock returns the array of blocks at an index
func (bc *Blockchain) GetBlock(index uint64) ([]byte, error) {
	return bc.BlockDb.Get([]byte(strconv.Itoa(int(index))), nil)
}

// write checks the channel for data to write and writes it to the socket
func (c *client) write() {
	for {
		toWrite := <-c.send

		_ = c.conn.WriteMessage(websocket.BinaryMessage, toWrite)
	}
}
