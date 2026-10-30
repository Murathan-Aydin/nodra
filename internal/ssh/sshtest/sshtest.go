// Package sshtest fournit un serveur SSH en mémoire pour les tests : il exécute les commandes
// localement (sh -c) ou via un gestionnaire personnalisé.
package sshtest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// Handler exécute une commande et renvoie son code de sortie.
type Handler func(cmd string, stdout, stderr io.Writer) int

// Local exécute la commande sur la machine de test.
func Local(cmd string, stdout, stderr io.Writer) int {
	c := exec.Command("sh", "-c", cmd)
	c.Stdout, c.Stderr = stdout, stderr
	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 127
	}
	return 0
}

type Server struct {
	Host    string
	Port    int
	HostKey gossh.PublicKey

	mu       sync.Mutex
	handler  Handler
	Commands []string // commandes reçues, dans l'ordre
	ln       net.Listener
}

// SetHandler remplace le gestionnaire de commandes.
func (s *Server) SetHandler(h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = h
}

// Received renvoie une copie des commandes reçues.
func (s *Server) Received() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.Commands...)
}

// New démarre un serveur qui accepte la clé publique authorized (format authorized_keys).
func New(t testing.TB, authorized string, h Handler) *Server {
	t.Helper()
	allowed, _, _, _, err := gossh.ParseAuthorizedKey([]byte(authorized))
	if err != nil {
		t.Fatal(err)
	}
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, _ := gossh.NewSignerFromKey(hostPriv)
	cfg := &gossh.ServerConfig{PublicKeyCallback: func(_ gossh.ConnMetadata, k gossh.PublicKey) (*gossh.Permissions, error) {
		if string(k.Marshal()) == string(allowed.Marshal()) {
			return nil, nil
		}
		return nil, io.EOF
	}}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, HostKey: hostSigner.PublicKey(), handler: h, ln: ln}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(nc, cfg)
		}
	}()
	return s
}

// AuthorizedHostKey renvoie la clé d'hôte au format authorized_keys.
func (s *Server) AuthorizedHostKey() string {
	return strings.TrimSpace(string(gossh.MarshalAuthorizedKey(s.HostKey)))
}

func (s *Server) serve(nc net.Conn, cfg *gossh.ServerConfig) {
	_, chans, reqs, err := gossh.NewServerConn(nc, cfg)
	if err != nil {
		nc.Close()
		return
	}
	go gossh.DiscardRequests(reqs)
	for nch := range chans {
		if nch.ChannelType() != "session" {
			_ = nch.Reject(gossh.UnknownChannelType, "type non pris en charge")
			continue
		}
		ch, chReqs, err := nch.Accept()
		if err != nil {
			continue
		}
		go s.session(ch, chReqs)
	}
}

func (s *Server) session(ch gossh.Channel, reqs <-chan *gossh.Request) {
	defer ch.Close()
	for req := range reqs {
		switch req.Type {
		case "exec":
			if len(req.Payload) < 4 {
				_ = req.Reply(false, nil)
				return
			}
			n := binary.BigEndian.Uint32(req.Payload)
			cmd := string(req.Payload[4 : 4+n])
			_ = req.Reply(true, nil)
			s.mu.Lock()
			s.Commands = append(s.Commands, cmd)
			h := s.handler
			s.mu.Unlock()
			code := h(cmd, ch, ch.Stderr())
			status := make([]byte, 4)
			binary.BigEndian.PutUint32(status, uint32(code))
			_, _ = ch.SendRequest("exit-status", false, status)
			return
		case "signal":
			return
		default:
			_ = req.Reply(false, nil)
		}
	}
}
