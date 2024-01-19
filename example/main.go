package main

import (
	"log"
	"net"
	"runtime"
	"strings"
	"syscall"

	pipgo "github.com/plumk97/pip-go"
	"github.com/plumk97/pip-go/types"
)

func main() {

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	pipgo.OutputIPDataCallback = outputIPDataCallback
	pipgo.NewTCPConnectCallback = newTCPConnectCallback
	pipgo.ReceiveUDPDataCallback = receiveUDPDataCallback
	pipgo.ReceiveICMPDataCallback = receiveICMPDataCallback

	createInterface()
}

func GetLocalIP() net.IP {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Println(err)
		return nil
	}

	for _, iface := range interfaces {
		if iface.Name == tunIface.Name() {
			continue
		}

		if runtime.GOOS != "windows" && !strings.HasPrefix(iface.Name, "en") {
			continue
		}

		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				ipnet, ok := addr.(*net.IPNet)
				if ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
					return ipnet.IP.To4()
				}
			}
		}
	}
	return nil
}

func outputIPDataCallback(buf *pipgo.Buffer) {
	b := make([]byte, buf.TotalLen())

	offset := 0
	for q := buf; q != nil; q = q.Next() {
		copy(b[offset:], q.Payload())
		offset += len(q.Payload())
	}

	tunIface.Write(b)
}

func receiveICMPDataCallback(data []byte, srcIP, dstIP net.IP, ttl uint8) {
	localIP := GetLocalIP()
	if localIP == nil {
		return
	}

	conn, err := net.DialIP("ip:icmp", &net.IPAddr{IP: localIP}, &net.IPAddr{IP: dstIP})
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, types.IPPROTO_ICMP)
	conn.Write(data)

	if runtime.GOOS == "windows" {
		b := make([]byte, 1024)
		n, _ := conn.Read(b)

		// 返回的是完整的IP包
		if n > 0 {
			pipgo.ICMPOutput(b[20:n], dstIP, srcIP)
			// tunIface.Write(b[:n])
		}
	}
}

func receiveUDPDataCallback(data []byte, srcIP net.IP, srcPort uint16, dstIP net.IP, dstPort uint16) {

	if dstIP[0] >= 224 || dstIP[len(dstIP)-1] == 255 {
		// 过滤D类和E类地址 过滤广播
		return
	}

	localIP := GetLocalIP()
	if localIP == nil {
		return
	}

	go func() {
		conn, err := net.DialUDP("udp",
			&net.UDPAddr{IP: localIP},
			&net.UDPAddr{IP: dstIP, Port: int(dstPort)})
		if err != nil {
			log.Println(err)
			return
		}
		defer conn.Close()

		conn.Write(data)

		bytes := [2048]byte{}
		n, err := conn.Read(bytes[:])
		if err != nil {
			log.Println(err)
			return
		}

		pipgo.UDPOutput(bytes[:n], dstIP, dstPort, srcIP, srcPort)
	}()
}

func newTCPConnectCallback(tcp *pipgo.TCP, handshakeData []byte) {
	localIP := GetLocalIP()
	if localIP == nil {
		tcp.Close()
		return
	}

	conn, err := net.DialTCP("tcp", &net.TCPAddr{
		// IP: localIP,
		IP: net.IP{127, 0, 0, 1},
	}, &net.TCPAddr{
		// IP:   tcp.IPHeader().Dst,
		IP:   net.IP{127, 0, 0, 1},
		Port: int(tcp.DstPort()),
	})

	if err != nil {
		tcp.Close()
		log.Println(err)
		return
	}

	onceRead := func() {
		buf := make([]byte, 65535<<tcp.OppWindShift())
		len, err := conn.Read(buf)
		if len <= 0 || err != nil {
			log.Println(err)
			tcp.Close()
		} else {
			tcp.Write(buf[:len])
		}
	}

	tcp.ReceivedCallback = func(tcp *pipgo.TCP, data []byte) {
		conn.Write(data)
		tcp.Received(uint16(len(data)))
	}

	tcp.WrittenCallback = func(tcp *pipgo.TCP, writtenLen int, hasPush, isDrop bool) {
		if hasPush || writtenLen == 0 {
			go onceRead()
		}
	}

	tcp.ClosedCallback = func(tcp *pipgo.TCP, arg any) {
		log.Println("ClosedCallback")
		conn.Close()
	}

	tcp.ConnectedCallback = func(tcp *pipgo.TCP) {
		log.Println("ConnectedCallback")
		go onceRead()
	}
	tcp.Connected(handshakeData)
}
