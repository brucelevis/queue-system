package server

import (
	"fmt"
	"log"
	"net"
	"os"
	def "thinkerchi/queue-system/define"
	que "thinkerchi/queue-system/queue"
	"time"
)

var (
	Ip   string
	Port string
)

func Run() {
	opt := fmt.Sprintf("%s:%s", Ip, Port)
	l, err := net.Listen("tcp", opt)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return

	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go NewHandler(conn).Handle()
	}
}

func NewHandler(conn net.Conn) *Handler {
	return &Handler{
		conn:    conn,
		stop:    make(chan struct{}),
		timeout: 2,
	}
}

type Handler struct {
	conn    net.Conn
	timeout int64
	stop    chan struct{}
}

func (h *Handler) Stop() {
	h.stop <- struct{}{}
}

func (h *Handler) Handle() {
	defer h.conn.Close()

	clientInfo, err := h.InitPacket()
	if err != nil {
		log.Println(err)
		return
	}

	go h.WriteBack(clientInfo)

	h.KeepReading(clientInfo.Id)

}

// 将更新后的信息实时向客户端写入
func (h *Handler) WriteBack(clientInfo *def.ClientInfo) {
	for {
		select {
		case nofityInfo := <-clientInfo.NotifyInfoChan:
			if err := h.WritePacket(&nofityInfo); err != nil {
				return
			}
		case <-h.stop:
			return
		}
	}
}

// 不断从客户端读取信息
// OPEN命令是客户端连接服务器发送的第一个命令
// SHUT、QUIT命令客户端并没有发送，因为没有未客户端提供发送这两个命令的入口...
// 客户端退出是由客户端放动断开链接实现的....
func (h *Handler) KeepReading(id string) {
	defer func() {
		h.Stop()

	}()
	for {
		readInfo, err := h.ReadPacket()
		if err != nil {
			go func() {
				que.QuitChan <- id
			}()
			return
		}

		switch readInfo.Cmd {
		case "SHUT":
			go func() {
				que.QuitQueueChan <- readInfo.Id
			}()
			log.Printf("user %s is quitting queuing\n", readInfo.Id)
		case "QUIT":
			go func() {
				que.QuitGameChan <- struct{}{}
			}()
			log.Printf("user %s is quitting game\n", readInfo.Id)
		default:
			log.Printf("Unknown cmd %v\n", readInfo)
		}

	}
}

func (h *Handler) InitPacket() (clientInfo *def.ClientInfo, err error) {
	readInfo, err := h.ReadPacket()
	if err != nil {
		log.Printf("read conn error:%v\n", err)
		return
	}
	if readInfo.Cmd != "OPEN" {
		err = fmt.Errorf("expected %s, got %s", "OPEN", readInfo.Cmd)
		log.Printf("%#v", *readInfo)
		return
	}
	clientInfo = &def.ClientInfo{
		Id:             readInfo.Id,
		NotifyInfoChan: make(chan def.NofityInfo),
	}

	go func() {
		que.EnqueueChan <- *clientInfo
	}()

	return
}

func (h *Handler) ReadPacket() (content *def.ReadInfo, err error) {
	rb := make([]byte, 36)
	n, err := h.conn.Read(rb)
	if err != nil {
		return
	} else if n != 36 {
		err = fmt.Errorf("read not enough bytes")
		return
	}

	content = new(def.ReadInfo)
	content.ReadFromBytes(rb)

	return
}

func (h *Handler) WritePacket(info *def.NofityInfo) (err error) {
	rb := info.ToBytes()
	h.conn.SetWriteDeadline(time.Now().Add(time.Second * time.Duration(h.timeout)))
	_, err = h.conn.Write(rb)

	return
}
