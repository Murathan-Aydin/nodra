package deploy

import (
	"fmt"
	"strings"
)

// splitArgs découpe une ligne de commande en respectant les guillemets simples et doubles.
func splitArgs(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	var quote rune
	inArg := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inArg = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inArg {
				out = append(out, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("guillemet non fermé")
	}
	if inArg {
		out = append(out, cur.String())
	}
	return out, nil
}

func appendList(svc map[string]any, key, v string) {
	l, _ := svc[key].([]string)
	svc[key] = append(l, v)
}

// ApplyRunOptions convertit les options `docker run` personnalisées (sous-ensemble courant, comme Coolify)
// en champs compose.
func ApplyRunOptions(svc map[string]any, opts string) error {
	args, err := splitArgs(opts)
	if err != nil {
		return err
	}
	for i := 0; i < len(args); i++ {
		name, val, hasVal := strings.Cut(args[i], "=")
		next := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("option %s sans valeur", name)
			}
			i++
			return args[i], nil
		}
		switch name {
		case "--privileged":
			svc["privileged"] = true
		case "--init":
			svc["init"] = true
		case "--read-only":
			svc["read_only"] = true
		case "--cap-add", "--cap-drop", "--device", "--add-host", "--security-opt", "--dns", "--group-add", "--tmpfs":
			v, err := next()
			if err != nil {
				return err
			}
			key := map[string]string{"--cap-add": "cap_add", "--cap-drop": "cap_drop", "--device": "devices", "--add-host": "extra_hosts",
				"--security-opt": "security_opt", "--dns": "dns", "--group-add": "group_add", "--tmpfs": "tmpfs"}[name]
			appendList(svc, key, v)
		case "--shm-size", "--hostname", "--user", "--pid", "--ipc", "--entrypoint", "--workdir", "-w", "--stop-signal", "--runtime":
			v, err := next()
			if err != nil {
				return err
			}
			key := map[string]string{"--shm-size": "shm_size", "--hostname": "hostname", "--user": "user", "--pid": "pid",
				"--ipc": "ipc", "--entrypoint": "entrypoint", "--workdir": "working_dir", "-w": "working_dir",
				"--stop-signal": "stop_signal", "--runtime": "runtime"}[name]
			svc[key] = v
		case "--ulimit":
			v, err := next()
			if err != nil {
				return err
			}
			k, lim, ok := strings.Cut(v, "=")
			if !ok {
				return fmt.Errorf("--ulimit attend nom=soft[:hard]")
			}
			ul, _ := svc["ulimits"].(map[string]any)
			if ul == nil {
				ul = map[string]any{}
			}
			soft, hard, two := strings.Cut(lim, ":")
			if two {
				ul[k] = map[string]any{"soft": soft, "hard": hard}
			} else {
				ul[k] = lim
			}
			svc["ulimits"] = ul
		case "--sysctl":
			v, err := next()
			if err != nil {
				return err
			}
			k, sv, _ := strings.Cut(v, "=")
			sc, _ := svc["sysctls"].(map[string]any)
			if sc == nil {
				sc = map[string]any{}
			}
			sc[k] = sv
			svc["sysctls"] = sc
		case "--gpus":
			if _, err := next(); err != nil {
				return err
			}
			svc["deploy"] = map[string]any{"resources": map[string]any{"reservations": map[string]any{"devices": []any{
				map[string]any{"driver": "nvidia", "count": "all", "capabilities": []string{"gpu"}}}}}}
		default:
			return fmt.Errorf("option docker run non prise en charge : %s", name)
		}
	}
	return nil
}
