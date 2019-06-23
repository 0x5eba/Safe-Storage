package client

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	protoBlockchain "../../protobufs/build/blockchain"
	"github.com/golang/protobuf/proto"
	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

type client struct {
	conn      *websocket.Conn
	send      chan []byte
	readOther chan []byte
	wg        sync.WaitGroup
	isOpen    bool
}

func UploadFile(serverIP, pathFile string) {
	c, err := ConnectServer(serverIP)
	if err != nil {
		log.Error(err)
	}
	c.SendData(protoBlockchain.Envelope_UPLOAD, pathFile)
}

func CheckFileExist(serverIP, pathFile string) {
	c, err := ConnectServer(serverIP)
	if err != nil {
		log.Error(err)
	}
	c.SendData(protoBlockchain.Envelope_EXIST, pathFile)
}

func (c *client) SendData(t protoBlockchain.Envelope_ContentType, pathFile string) {
	fileStat, err := os.Stat(pathFile)
	if err != nil {
		log.Error(err)
	}

	dat, err := ioutil.ReadFile(pathFile)
	if err != nil {
		log.Error(err)
	}

	infoFile := &protoBlockchain.File{
		Name:             []byte(fileStat.Name()),
		Size:             []byte(strconv.Itoa(int(fileStat.Size()))),
		LastModification: []byte(fileStat.ModTime().String()),
		Data:             dat,
	}

	infoFileByte, err := proto.Marshal(infoFile)
	if err != nil {
		log.Error(err)
	}

	env := &protoBlockchain.Envelope{
		Type: t,
		Data: infoFileByte,
	}
	data, err := proto.Marshal(env)
	if err != nil {
		log.Error(err)
	}

	if c.isOpen {
		c.send <- data
	}

	time.Sleep(1 * time.Second)
}

// ConnectServer is to connect to clients
func ConnectServer(ip string) (*client, error) {
	dial := websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 5 * time.Second,
	}

	conn, _, err := dial.Dial(fmt.Sprintf("ws://%s/ws", ip), nil)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	c := client{
		conn:      conn,
		send:      make(chan []byte, 256),
		readOther: make(chan []byte, 256),
		isOpen:    true,
	}

	go c.read()
	go c.write()

	return &c, nil
}

// read reads data from the socket and handles it
func (c *client) read() {
	// Unregister if the node dies
	defer func() {
		log.Info("Server died")
		c.isOpen = false
		c.conn.Close()
	}()

	for {
		msgType, msg, err := c.conn.ReadMessage()
		if err != nil {
			continue
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

		go func() {
			switch pb.GetType() {

			case protoBlockchain.Envelope_UPLOAD:
				if len(pb.GetData()) > 0 {
					log.Info("File Uploaded")
					f, err := os.Create("tmp")
					if err != nil {
						log.Error(err)
						return
					}
					// f.WriteString(string(pb.GetData()))
					f.WriteString("yes")
				} else {
					f, err := os.Create("tmp")
					if err != nil {
						log.Error(err)
						return
					}
					f.WriteString("no")
				}

			case protoBlockchain.Envelope_EXIST:
				if len(pb.GetData()) > 0 {
					log.Info("File Exist")
					f, err := os.Create("tmp")
					if err != nil {
						log.Error(err)
						return
					}
					f.WriteString(string(pb.GetData()))
				} else {
					log.Info("File doesn't Exist")
					f, err := os.Create("tmp")
					if err != nil {
						log.Error(err)
						return
					}
					f.WriteString("no")
				}
			}
		}()
	}
}

// write checks the channel for data to write and writes it to the socket
func (c *client) write() {
	for {
		toWrite := <-c.send
		_ = c.conn.WriteMessage(websocket.BinaryMessage, toWrite)
	}
}
