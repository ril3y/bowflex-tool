package adb

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"strings"
	"time"
)

// ADB wire protocol — direct device connection, no adb server needed.

const (
	cmdCNXN = 0x4e584e43 // "CNXN"
	cmdAUTH = 0x48545541 // "AUTH"
	cmdOPEN = 0x4e45504f // "OPEN"
	cmdOKAY = 0x59414b4f // "OKAY"
	cmdWRTE = 0x45545257 // "WRTE"
	cmdCLSE = 0x45534c43 // "CLSE"

	adbVersion = 0x01000001
	maxPayload = 256 * 1024
	authToken  = 1
	authSig    = 2
	authRSAKey = 3
)

type adbMessage struct {
	Command uint32
	Arg0    uint32
	Arg1    uint32
	DataLen uint32
	DataCRC uint32
	Magic   uint32
}

// Conn represents a direct connection to a device's adbd.
type Conn struct {
	conn    net.Conn
	localID uint32
}

// Cached RSA key
var adbKey *rsa.PrivateKey
var authMessageShown bool

func getOrCreateKey() *rsa.PrivateKey {
	if adbKey != nil {
		return adbKey
	}

	// Try to load existing key from ~/.android/adbkey
	home, _ := os.UserHomeDir()
	keyPath := home + "/.android/adbkey"

	if data, err := os.ReadFile(keyPath); err == nil {
		block, _ := pem.Decode(data)
		if block != nil {
			if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
				adbKey = key
				return adbKey
			}
			if keyI, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
				if key, ok := keyI.(*rsa.PrivateKey); ok {
					adbKey = key
					return adbKey
				}
			}
		}
	}

	// Generate new key
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	adbKey = key

	// Save for reuse
	os.MkdirAll(home+"/.android", 0700)
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	os.WriteFile(keyPath, privPEM, 0600)

	return adbKey
}

// adbPubKey returns the public key in Android ADB format (base64 of struct + " hostname\0")
func adbPubKey(key *rsa.PrivateKey) []byte {
	pub := &key.PublicKey
	n := pub.N
	nBytes := n.Bytes() // big-endian
	nWords := len(nBytes) / 4
	if len(nBytes)%4 != 0 {
		nWords++
	}

	// Convert modulus to little-endian uint32 array
	nLE := make([]byte, nWords*4)
	for i, b := range nBytes {
		nLE[len(nBytes)-1-i] = b
	}

	// Compute n0inv = -(n^-1 mod 2^32)
	n0 := uint64(binary.LittleEndian.Uint32(nLE[0:4]))
	inv := modInverse32(n0)
	n0inv := uint32((-int64(inv)) & 0xFFFFFFFF)

	// Compute R^2 mod n  (R = 2^(nWords*32))
	r := new(big.Int).Lsh(big.NewInt(1), uint(nWords*32))
	rr := new(big.Int).Mul(r, r)
	rr.Mod(rr, n)
	rrBytes := rr.Bytes()
	rrLE := make([]byte, nWords*4)
	for i, b := range rrBytes {
		rrLE[len(rrBytes)-1-i] = b
	}

	// Build struct: len(4) + n0inv(4) + n(nWords*4) + rr(nWords*4) + e(4)
	bufSize := 4 + 4 + nWords*4 + nWords*4 + 4
	buf := make([]byte, bufSize)
	binary.LittleEndian.PutUint32(buf[0:4], uint32(nWords))
	binary.LittleEndian.PutUint32(buf[4:8], n0inv)
	copy(buf[8:8+nWords*4], nLE)
	copy(buf[8+nWords*4:8+nWords*4*2], rrLE)
	binary.LittleEndian.PutUint32(buf[8+nWords*4*2:], uint32(pub.E))

	encoded := base64.StdEncoding.EncodeToString(buf)
	hostname, _ := os.Hostname()
	return []byte(encoded + " " + hostname + "\x00")
}

func modInverse32(n uint64) uint64 {
	x := uint64(1)
	for i := 0; i < 31; i++ {
		x = (x * (2 - n*x)) & 0xFFFFFFFF
	}
	return x
}

// Connect establishes a direct ADB connection to a device.
func Connect(host string, port int, logFn ...func(string)) (*Conn, error) {
	logMsg := func(msg string) {
		if len(logFn) > 0 && logFn[0] != nil {
			logFn[0](msg)
		}
	}
	_ = logMsg
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("TCP connect failed: %w", err)
	}

	ac := &Conn{conn: conn, localID: 1}

	// Send CNXN
	banner := "host::features=shell_v2,cmd,stat_v2,ls_v2,fixed_push_mkdir,apex,abb,fixed_push_symlink_timestamp,abb_exec,remount_shell,track_app,sendrecv_v2,sendrecv_v2_brotli,sendrecv_v2_lz4,sendrecv_v2_zstd,sendrecv_v2_dry_run_send,openscreen_mdns,push_sync"
	if err := ac.sendMsg(cmdCNXN, adbVersion, maxPayload, []byte(banner+"\x00")); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send CNXN failed: %w", err)
	}

	key := getOrCreateKey()
	authAttempts := 0

	// Read response — with shorter timeout for auth loop
	for {
		msg, data, err := ac.readMsg()
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("read response failed: %w", err)
		}

		switch msg.Command {
		case cmdCNXN:
			_ = data
			return ac, nil

		case cmdAUTH:
			authAttempts++
			if authAttempts > 3 {
				conn.Close()
				return nil, fmt.Errorf("auth failed after %d attempts — device may need manual ADB authorization", authAttempts)
			}

			if msg.Arg0 == authToken {
				signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA1, data)
				if err == nil {
					if err := ac.sendMsg(cmdAUTH, authSig, 0, signature); err != nil {
						conn.Close()
						return nil, fmt.Errorf("send auth signature failed: %w", err)
					}
					msg2, data2, err := ac.readMsg()
					if err != nil {
						conn.Close()
						return nil, fmt.Errorf("auth response failed: %w", err)
					}
					if msg2.Command == cmdCNXN {
						_ = data2
						return ac, nil
					}
					if msg2.Command == cmdAUTH {
						pubKeyData := adbPubKey(key)
						if err := ac.sendMsg(cmdAUTH, authRSAKey, 0, pubKeyData); err != nil {
							conn.Close()
							return nil, fmt.Errorf("send public key failed: %w", err)
						}
						if !authMessageShown {
							logMsg("Waiting for USB debugging authorization on bike screen...")
							logMsg("Tap ALLOW on the bike (check 'Always allow' box)")
							authMessageShown = true
						}
						conn.SetReadDeadline(time.Now().Add(60 * time.Second))
						continue
					}
				}
			}
			conn.Close()
			return nil, fmt.Errorf("unexpected AUTH type: %d", msg.Arg0)

		default:
			conn.Close()
			return nil, fmt.Errorf("unexpected response: 0x%08x", msg.Command)
		}
	}
}

// IsADBPort checks if a TCP connection on port 5555 is actually an ADB daemon.
func IsADBPort(ip string) bool {
	c, err := net.DialTimeout("tcp", ip+":5555", 500*time.Millisecond)
	if err != nil {
		return false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(1 * time.Second))

	banner := []byte("host::\x00")
	var buf [24]byte
	binary.LittleEndian.PutUint32(buf[0:], cmdCNXN)
	binary.LittleEndian.PutUint32(buf[4:], adbVersion)
	binary.LittleEndian.PutUint32(buf[8:], maxPayload)
	binary.LittleEndian.PutUint32(buf[12:], uint32(len(banner)))
	var crc uint32
	for _, b := range banner {
		crc += uint32(b)
	}
	binary.LittleEndian.PutUint32(buf[16:], crc)
	binary.LittleEndian.PutUint32(buf[20:], cmdCNXN^0xFFFFFFFF)
	if _, err := c.Write(buf[:]); err != nil {
		return false
	}
	if _, err := c.Write(banner); err != nil {
		return false
	}

	var resp [24]byte
	if _, err := io.ReadFull(c, resp[:]); err != nil {
		return false
	}
	cmd := binary.LittleEndian.Uint32(resp[0:])
	return cmd == cmdCNXN || cmd == cmdAUTH
}

func (ac *Conn) Close() {
	if ac.conn != nil {
		ac.conn.Close()
	}
}

// Shell runs a command and returns output.
func (ac *Conn) Shell(cmd string) (string, error) {
	localID := ac.nextID()
	dest := "shell:" + cmd + "\x00"

	if err := ac.sendMsg(cmdOPEN, localID, 0, []byte(dest)); err != nil {
		return "", err
	}

	remoteID, err := ac.waitOkay(localID)
	if err != nil {
		return "", fmt.Errorf("shell open failed: %w", err)
	}

	var buf bytes.Buffer
	for {
		msg, data, err := ac.readMsg()
		if err != nil {
			break
		}
		switch msg.Command {
		case cmdWRTE:
			buf.Write(data)
			ac.sendMsg(cmdOKAY, localID, remoteID, nil)
		case cmdCLSE:
			return buf.String(), nil
		}
	}
	return buf.String(), nil
}

// Push sends a file to the device.
func (ac *Conn) Push(localPath, remotePath string, mode uint32) error {
	data, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("read local file: %w", err)
	}

	localID := ac.nextID()

	if err := ac.sendMsg(cmdOPEN, localID, 0, []byte("sync:\x00")); err != nil {
		return err
	}

	remoteID, err := ac.waitOkay(localID)
	if err != nil {
		return fmt.Errorf("sync open failed: %w", err)
	}

	sendPath := fmt.Sprintf("%s,%d", remotePath, mode)
	sendHeader := make([]byte, 8)
	copy(sendHeader[0:4], "SEND")
	binary.LittleEndian.PutUint32(sendHeader[4:8], uint32(len(sendPath)))
	sendPayload := append(sendHeader, []byte(sendPath)...)

	if err := ac.sendMsg(cmdWRTE, localID, remoteID, sendPayload); err != nil {
		return err
	}
	if _, err := ac.waitOkay(localID); err != nil {
		return fmt.Errorf("SEND ack failed: %w", err)
	}

	const chunkSize = 64 * 1024
	for offset := 0; offset < len(data); offset += chunkSize {
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]

		dataHeader := make([]byte, 8)
		copy(dataHeader[0:4], "DATA")
		binary.LittleEndian.PutUint32(dataHeader[4:8], uint32(len(chunk)))
		payload := append(dataHeader, chunk...)

		if err := ac.sendMsg(cmdWRTE, localID, remoteID, payload); err != nil {
			return err
		}
		if _, err := ac.waitOkay(localID); err != nil {
			return fmt.Errorf("DATA ack failed: %w", err)
		}
	}

	doneMsg := make([]byte, 8)
	copy(doneMsg[0:4], "DONE")
	binary.LittleEndian.PutUint32(doneMsg[4:8], uint32(time.Now().Unix()))

	if err := ac.sendMsg(cmdWRTE, localID, remoteID, doneMsg); err != nil {
		return err
	}
	if _, err := ac.waitOkay(localID); err != nil {
		return fmt.Errorf("DONE ack failed: %w", err)
	}

	msg, respData, err := ac.readMsg()
	if err != nil {
		return err
	}
	if msg.Command == cmdWRTE && len(respData) >= 4 {
		status := string(respData[0:4])
		if status == "OKAY" {
			ac.sendMsg(cmdOKAY, localID, remoteID, nil)
			ac.sendMsg(cmdCLSE, localID, remoteID, nil)
			return nil
		} else if status == "FAIL" && len(respData) >= 8 {
			failLen := binary.LittleEndian.Uint32(respData[4:8])
			failMsg := string(respData[8 : 8+failLen])
			return fmt.Errorf("push failed: %s", failMsg)
		}
	}

	ac.sendMsg(cmdCLSE, localID, remoteID, nil)
	return nil
}

// Pull downloads a file from the device.
func (ac *Conn) Pull(remotePath, localPath string) error {
	localID := ac.nextID()

	if err := ac.sendMsg(cmdOPEN, localID, 0, []byte("sync:\x00")); err != nil {
		return err
	}

	remoteID, err := ac.waitOkay(localID)
	if err != nil {
		return fmt.Errorf("sync open failed: %w", err)
	}

	recvHeader := make([]byte, 8)
	copy(recvHeader[0:4], "RECV")
	binary.LittleEndian.PutUint32(recvHeader[4:8], uint32(len(remotePath)))
	recvPayload := append(recvHeader, []byte(remotePath)...)

	if err := ac.sendMsg(cmdWRTE, localID, remoteID, recvPayload); err != nil {
		return err
	}
	if _, err := ac.waitOkay(localID); err != nil {
		return fmt.Errorf("RECV ack failed: %w", err)
	}

	var buf bytes.Buffer
	for {
		msg, data, err := ac.readMsg()
		if err != nil {
			return err
		}
		if msg.Command == cmdWRTE && len(data) >= 8 {
			ac.sendMsg(cmdOKAY, localID, remoteID, nil)
			tag := string(data[0:4])
			if tag == "DATA" {
				dataLen := binary.LittleEndian.Uint32(data[4:8])
				buf.Write(data[8 : 8+dataLen])
			} else if tag == "DONE" {
				break
			} else if tag == "FAIL" {
				failLen := binary.LittleEndian.Uint32(data[4:8])
				return fmt.Errorf("pull failed: %s", string(data[8:8+failLen]))
			}
		} else if msg.Command == cmdCLSE {
			break
		}
	}

	ac.sendMsg(cmdCLSE, localID, remoteID, nil)
	return os.WriteFile(localPath, buf.Bytes(), 0644)
}

// Install pushes an APK to /data/local/tmp and runs pm install.
func (ac *Conn) Install(localPath string) error {
	tmpPath := "/data/local/tmp/_install.apk"
	if err := ac.Push(localPath, tmpPath, 0644); err != nil {
		return fmt.Errorf("push APK: %w", err)
	}
	out, err := ac.Shell("pm install -r " + tmpPath)
	if err != nil {
		return fmt.Errorf("pm install: %w", err)
	}
	if !strings.Contains(out, "Success") {
		return fmt.Errorf("install failed: %s", strings.TrimSpace(out))
	}
	ac.Shell("rm " + tmpPath)
	return nil
}

// --- Wire protocol ---

func (ac *Conn) sendMsg(cmd, arg0, arg1 uint32, data []byte) error {
	msg := adbMessage{
		Command: cmd,
		Arg0:    arg0,
		Arg1:    arg1,
		DataLen: uint32(len(data)),
		DataCRC: checksum(data),
		Magic:   cmd ^ 0xFFFFFFFF,
	}
	ac.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := binary.Write(ac.conn, binary.LittleEndian, msg); err != nil {
		return err
	}
	if len(data) > 0 {
		_, err := ac.conn.Write(data)
		return err
	}
	return nil
}

func (ac *Conn) readMsg() (adbMessage, []byte, error) {
	var msg adbMessage
	ac.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	if err := binary.Read(ac.conn, binary.LittleEndian, &msg); err != nil {
		return msg, nil, err
	}
	var data []byte
	if msg.DataLen > 0 {
		data = make([]byte, msg.DataLen)
		if _, err := io.ReadFull(ac.conn, data); err != nil {
			return msg, nil, err
		}
	}
	return msg, data, nil
}

func (ac *Conn) waitOkay(localID uint32) (uint32, error) {
	for {
		msg, _, err := ac.readMsg()
		if err != nil {
			return 0, err
		}
		switch msg.Command {
		case cmdOKAY:
			return msg.Arg0, nil
		case cmdCLSE:
			return 0, fmt.Errorf("connection closed by device")
		case cmdWRTE:
			ac.sendMsg(cmdOKAY, localID, msg.Arg0, nil)
		default:
			return 0, fmt.Errorf("expected OKAY, got 0x%08x", msg.Command)
		}
	}
}

func (ac *Conn) nextID() uint32 {
	ac.localID++
	return ac.localID
}

func checksum(data []byte) uint32 {
	var sum uint32
	for _, b := range data {
		sum += uint32(b)
	}
	return sum
}
