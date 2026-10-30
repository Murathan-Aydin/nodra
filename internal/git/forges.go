package git

import (
	"context"
	"encoding/json"
)

func gitlabRepos(ctx context.Context, s Source) ([]Repo, error) {
	return paginate(ctx, s.APIURL+"/projects?membership=true&archived=false&per_page=100&order_by=last_activity_at", "Bearer "+s.AccessToken,
		func(b []byte) ([]Repo, error) {
			var list []struct {
				PathWithNamespace string `json:"path_with_namespace"`
				Path              string `json:"path"`
				Visibility        string `json:"visibility"`
				DefaultBranch     string `json:"default_branch"`
				WebURL            string `json:"web_url"`
			}
			if err := json.Unmarshal(b, &list); err != nil {
				return nil, err
			}
			out := make([]Repo, len(list))
			for i, r := range list {
				out[i] = Repo{r.PathWithNamespace, r.Path, r.Visibility != "public", r.DefaultBranch, r.WebURL}
			}
			return out, nil
		})
}

func giteaRepos(ctx context.Context, s Source) ([]Repo, error) {
	return paginate(ctx, s.APIURL+"/user/repos?limit=50", "token "+s.AccessToken, func(b []byte) ([]Repo, error) {
		var list []struct {
			FullName      string `json:"full_name"`
			Name          string `json:"name"`
			Private       bool   `json:"private"`
			DefaultBranch string `json:"default_branch"`
			HTMLURL       string `json:"html_url"`
		}
		if err := json.Unmarshal(b, &list); err != nil {
			return nil, err
		}
		out := make([]Repo, len(list))
		for i, r := range list {
			out[i] = Repo{r.FullName, r.Name, r.Private, r.DefaultBranch, r.HTMLURL}
		}
		return out, nil
	})
}
