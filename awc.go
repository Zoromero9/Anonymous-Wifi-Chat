package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	broadcastPort = "9999"
	chatPort      = "8080"
)

type PeerList struct {
	mu    sync.Mutex
	conns []net.Conn
}

func (p *PeerList) Add(conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.conns = append(p.conns, conn)
}

func (p *PeerList) Remove(conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	newConns := make([]net.Conn, 0)
	for _, c := range p.conns {
		if c != conn {
			newConns = append(newConns, c)
		}
	}
	p.conns = newConns
}

func (p *PeerList) Broadcast(msg string, exclude net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, conn := range p.conns {
		if conn != exclude {
			fmt.Fprintln(conn, msg)
		}
	}
}

// Tambah Count() agar aman pakai lock
func (p *PeerList) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.conns)
}

func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	localIP := getLocalIP()

	fmt.Println("=== P2P CHAT - WIFI ===")
	fmt.Printf("IP Kamu: %s\n", localIP)
	fmt.Println("1. Buat Room")
	fmt.Println("2. Join Room")
	fmt.Print("Pilih menu (1/2): ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		createRoom(reader, localIP)
	case "2":
		joinRoom(reader, localIP)
	default:
		fmt.Println("Pilihan tidak valid.")
	}
}

func createRoom(reader *bufio.Reader, localIP string) {
	fmt.Print("Kode Room (maks 5 huruf): ")
	roomCode, _ := reader.ReadString('\n')
	roomCode = strings.TrimSpace(roomCode)

	fmt.Print("Nama kamu: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)

	peers := &PeerList{}

	// Broadcast keberadaan room terus menerus
	go broadcastRoom(roomCode, localIP)

	ln, err := net.Listen("tcp", ":"+chatPort)
	if err != nil {
		fmt.Println("Gagal membuat room:", err)
		return
	}
	defer ln.Close()

	fmt.Printf("\nRoom [%s] aktif! Menunggu teman...\n", roomCode)
	fmt.Println("(Ketik pesan dan Enter untuk kirim)\n")

	// Terima koneksi banyak joiner sekaligus
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				continue
			}
			peers.Add(conn)
			fmt.Printf("\n[System] Teman baru terhubung! Online: %d\n> ", peers.Count())
			go receiveMessages(conn, peers)
		}
	}()

	sendMessages(peers, name)
}

func broadcastRoom(roomCode, localIP string) {
	addr, err := net.ResolveUDPAddr("udp", "255.255.255.255:"+broadcastPort)
	if err != nil {
		return
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return
	}
	defer conn.Close()

	for {
		msg := fmt.Sprintf("ROOM:%s:IP:%s", roomCode, localIP)
		conn.Write([]byte(msg))
		time.Sleep(1 * time.Second)
	}
}

func joinRoom(reader *bufio.Reader, localIP string) {
	fmt.Print("Kode Room: ")
	roomCode, _ := reader.ReadString('\n')
	roomCode = strings.TrimSpace(roomCode)

	fmt.Print("Nama kamu: ")
	name, _ := reader.ReadString('\n')
	name = strings.TrimSpace(name)

	fmt.Print("IP Host (kosongkan untuk auto-cari): ")
	manualIP, _ := reader.ReadString('\n')
	manualIP = strings.TrimSpace(manualIP)

	var hostIP string
	if manualIP != "" {
		hostIP = manualIP
	} else {
		fmt.Println("Mencari room di jaringan WiFi...")
		hostIP = findRoom(roomCode, localIP)
	}

	if hostIP == "" {
		fmt.Println("Room tidak ditemukan.")
		return
	}

	conn, err := net.Dial("tcp", hostIP+":"+chatPort)
	if err != nil {
		fmt.Println("Gagal konek ke host:", err)
		return
	}

	fmt.Printf("Terhubung ke Room [%s]!\n", roomCode)
	fmt.Println("(Ketik pesan dan Enter untuk kirim)\n")

	peers := &PeerList{}
	peers.Add(conn)

	go receiveMessages(conn, peers)
	sendMessages(peers, name)
}

func findRoom(roomCode, localIP string) string {
	addr, err := net.ResolveUDPAddr("udp", ":"+broadcastPort)
	if err != nil {
		return ""
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return ""
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	buf := make([]byte, 256)

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Println("Timeout. Room tidak ditemukan.")
			return ""
		}

		msg := string(buf[:n])
		if strings.HasPrefix(msg, "ROOM:"+roomCode+":IP:") {
			parts := strings.Split(msg, ":")
			hostIP := parts[3]
			if hostIP != localIP {
				return hostIP
			}
		}
	}
}

func receiveMessages(conn net.Conn, peers *PeerList) {
	defer func() {
		peers.Remove(conn)
		conn.Close()
		fmt.Printf("\n[System] Teman disconnect. Online: %d\n> ", peers.Count())
	}()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		msg := scanner.Text()
		fmt.Printf("\r%s\n> ", msg)
		peers.Broadcast(msg, conn)
	}
}

func sendMessages(peers *PeerList, name string) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		msg := fmt.Sprintf("[%s]: %s", name, text)
		peers.Broadcast(msg, nil)
	}
}