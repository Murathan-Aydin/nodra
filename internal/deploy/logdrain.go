package deploy

import (
	"gopkg.in/yaml.v3"
)

// fluentdLogging : pilote de logs vers le log drain du serveur (Fluent Bit sur 127.0.0.1:24224).
// Asynchrone : un log drain arrêté ne bloque ni ne fait échouer le conteneur.
func fluentdLogging() map[string]any {
	return map[string]any{"driver": "fluentd", "options": map[string]any{
		"fluentd-address": "tcp://127.0.0.1:24224", "fluentd-async": "true", "fluentd-sub-second-precision": "true",
		"env": "NODRA_RESOURCE_UUID,NODRA_CONTAINER_NAME", "tag": "nodra.{{.Name}}",
	}}
}

// WithLogDrain ajoute le pilote de logs du log drain aux services d'un docker-compose qui n'en définissent pas.
func WithLogDrain(compose string) (string, error) {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(compose), &doc); err != nil {
		return "", err
	}
	services, _ := doc["services"].(map[string]any)
	for _, v := range services {
		if svc, ok := v.(map[string]any); ok {
			if _, set := svc["logging"]; !set {
				svc["logging"] = fluentdLogging()
			}
		}
	}
	out, err := yaml.Marshal(doc)
	return string(out), err
}
