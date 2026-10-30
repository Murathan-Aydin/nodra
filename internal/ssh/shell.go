package ssh

import (
	"context"
	"errors"
	"io"

	gossh "golang.org/x/crypto/ssh"
)

// Shell : session interactive avec pseudo-terminal (terminal web, suivi de logs).
// À la fermeture, sshd libère le terminal : les processus lancés reçoivent SIGHUP.
type Shell struct {
	sess   *gossh.Session
	Stdin  io.WriteCloser
	Stdout io.Reader // sorties standard et d'erreur mêlées par le terminal
}

// ShellOptions : taille initiale du terminal ; Raw désactive l'écho et la conversion des fins de ligne
// (flux de logs lus par programme plutôt qu'affichés dans un émulateur de terminal).
type ShellOptions struct {
	Rows, Cols int
	Raw        bool
}

// Shell ouvre une session avec pseudo-terminal et lance cmd (le shell de connexion si cmd est vide).
func (p *Pool) Shell(ctx context.Context, t Target, cmd string, o ShellOptions) (*Shell, error) {
	cl, err := p.client(ctx, t)
	if err != nil {
		return nil, err
	}
	sess, err := cl.NewSession()
	if err != nil {
		p.Drop(t)
		if cl, err = p.client(ctx, t); err != nil {
			return nil, err
		}
		if sess, err = cl.NewSession(); err != nil {
			return nil, err
		}
	}
	if o.Rows <= 0 || o.Cols <= 0 {
		o.Rows, o.Cols = 24, 80
	}
	modes := gossh.TerminalModes{gossh.TTY_OP_ISPEED: 14400, gossh.TTY_OP_OSPEED: 14400}
	if o.Raw {
		modes[gossh.ECHO], modes[gossh.ONLCR], modes[gossh.OPOST] = 0, 0, 0
	}
	s := &Shell{sess: sess}
	err = func() error {
		if err := sess.RequestPty("xterm-256color", o.Rows, o.Cols, modes); err != nil {
			return err
		}
		var err error
		if s.Stdin, err = sess.StdinPipe(); err != nil {
			return err
		}
		if s.Stdout, err = sess.StdoutPipe(); err != nil {
			return err
		}
		if cmd == "" {
			return sess.Shell()
		}
		return sess.Start(cmd)
	}()
	if err != nil {
		sess.Close()
		return nil, err
	}
	return s, nil
}

// Resize transmet la nouvelle taille du terminal.
func (s *Shell) Resize(rows, cols int) error {
	if rows <= 0 || cols <= 0 || rows > 1000 || cols > 1000 {
		return errors.New("taille de terminal invalide")
	}
	return s.sess.WindowChange(rows, cols)
}

// Wait attend la fin de la commande et renvoie son code de sortie (-1 si la session a été coupée).
func (s *Shell) Wait() int {
	err := s.sess.Wait()
	var exit *gossh.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.ExitStatus()
	default:
		return -1
	}
}

// Close coupe la session (le terminal distant est libéré).
func (s *Shell) Close() error {
	_ = s.sess.Signal(gossh.SIGHUP)
	return s.sess.Close()
}

// Follow lance cmd dans un terminal brut et copie sa sortie dans w jusqu'à la fin de la commande
// ou l'annulation du contexte (suivi de logs : docker logs -f).
func (p *Pool) Follow(ctx context.Context, t Target, cmd string, w io.Writer) (int, error) {
	sh, err := p.Shell(ctx, t, cmd, ShellOptions{Raw: true, Rows: 50, Cols: 500})
	if err != nil {
		return -1, err
	}
	defer sh.Close()
	stop := context.AfterFunc(ctx, func() { sh.Close() })
	defer stop()
	if _, err := io.Copy(w, sh.Stdout); err != nil { // lecteur parti : on coupe la commande
		sh.Close()
	}
	code := sh.Wait()
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	return code, nil
}
