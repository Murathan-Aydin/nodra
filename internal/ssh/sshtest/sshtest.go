// Package sshtest fournit un serveur SSH en mémoire pour les tests : il exécute les commandes
// localement (sh -c) ou via un gestionnaire personnalisé.
package sshtest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
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

// InputHandler reçoit aussi l'entrée standard de la commande.
type InputHandler func(cmd string, stdin io.Reader, stdout, stderr io.Writer) int

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
	inputH   InputHandler
	Stdins   []string // entrées standard reçues (scripts), dans l'ordre
	lastDone bool
	Commands []string // commandes reçues, dans l'ordre
	ptys     []*Pty
	ln       net.Listener
}

// SetHandler remplace le gestionnaire de commandes.
func (s *Server) SetHandler(h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = h
}

// SetInputHandler installe un gestionnaire qui reçoit l'entrée standard (prioritaire sur SetHandler).
func (s *Server) SetInputHandler(h InputHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inputH = h
}

// ReceivedStdins renvoie une copie des entrées standard reçues.
func (s *Server) ReceivedStdins() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.Stdins...)
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

// Pty : pseudo-terminal demandé par un client (taille initiale puis redimensionnements).
type Pty struct {
	Term    string
	Sizes   [][2]int // [lignes, colonnes]
	Command string
}

// Ptys renvoie une copie des pseudo-terminaux demandés.
func (s *Server) Ptys() []Pty {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Pty, len(s.ptys))
	for i, p := range s.ptys {
		out[i] = *p
		out[i].Sizes = append([][2]int(nil), p.Sizes...)
	}
	return out
}

func (s *Server) session(ch gossh.Channel, reqs <-chan *gossh.Request) {
	var once sync.Once
	closeCh := func() { once.Do(func() { ch.Close() }) }
	defer closeCh()
	var pty *Pty
	for req := range reqs {
		switch req.Type {
		case "pty-req", "window-change":
			if req.Type == "pty-req" {
				pty = &Pty{}
				if n := binary.BigEndian.Uint32(req.Payload); int(n)+12 <= len(req.Payload) {
					pty.Term = string(req.Payload[4 : 4+n])
					rest := req.Payload[4+n:]
					pty.Sizes = append(pty.Sizes, [2]int{int(binary.BigEndian.Uint32(rest[4:])), int(binary.BigEndian.Uint32(rest))})
				}
				s.mu.Lock()
				s.ptys = append(s.ptys, pty)
				s.mu.Unlock()
				_ = req.Reply(true, nil)
			} else if pty != nil && len(req.Payload) >= 8 {
				s.mu.Lock()
				pty.Sizes = append(pty.Sizes, [2]int{int(binary.BigEndian.Uint32(req.Payload[4:])), int(binary.BigEndian.Uint32(req.Payload))})
				s.mu.Unlock()
			}
		case "exec", "shell":
			cmd := ""
			if req.Type == "exec" {
				if len(req.Payload) < 4 {
					_ = req.Reply(false, nil)
					return
				}
				n := binary.BigEndian.Uint32(req.Payload)
				cmd = string(req.Payload[4 : 4+n])
			}
			_ = req.Reply(true, nil)
			s.mu.Lock()
			s.Commands = append(s.Commands, cmd)
			if pty != nil {
				pty.Command = cmd
			}
			h, ih := s.handler, s.inputH
			s.mu.Unlock()
			if ih == nil {
				ih = func(cmd string, stdin io.Reader, stdout, stderr io.Writer) int {
					if cmd == "sh -s" && h == nil {
						return 127
					}
					return runWithStdin(h, cmd, stdin, stdout, stderr)
				}
			}
			// Exécution en parallèle : les redimensionnements et signaux restent traités.
			go func() {
				code := ih(cmd, &recordingReader{ch, s}, ch, ch.Stderr())
				status := make([]byte, 4)
				binary.BigEndian.PutUint32(status, uint32(code))
				_, _ = ch.SendRequest("exit-status", false, status)
				closeCh()
			}()
		case "signal":
			return
		default:
			_ = req.Reply(false, nil)
		}
	}
}

// recordingReader enregistre l'entrée standard lue par le gestionnaire.
type recordingReader struct {
	r io.Reader
	s *Server
}

func (r *recordingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.s.mu.Lock()
		if len(r.s.Stdins) == 0 || r.s.lastDone {
			r.s.Stdins = append(r.s.Stdins, "")
			r.s.lastDone = false
		}
		r.s.Stdins[len(r.s.Stdins)-1] += string(p[:n])
		r.s.mu.Unlock()
	}
	if err == io.EOF {
		r.s.mu.Lock()
		r.s.lastDone = true
		r.s.mu.Unlock()
	}
	return n, err
}

// runWithStdin : pour le gestionnaire Local, transmet stdin à sh ; les autres gestionnaires l'ignorent.
func runWithStdin(h Handler, cmd string, stdin io.Reader, stdout, stderr io.Writer) int {
	if isLocal(h) {
		c := exec.Command("sh", "-c", cmd)
		c.Stdin, c.Stdout, c.Stderr = stdin, stdout, stderr
		if err := c.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			return 127
		}
		return 0
	}
	return h(cmd, stdout, stderr)
}

func isLocal(h Handler) bool {
	return fmt.Sprintf("%p", h) == fmt.Sprintf("%p", Handler(Local))
}
