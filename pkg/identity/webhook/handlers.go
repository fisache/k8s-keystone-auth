/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

type userInfo struct {
	Username string              `json:"username"`
	UID      string              `json:"uid"`
	Groups   []string            `json:"groups"`
	Extra    map[string][]string `json:"extra"`
}
type status struct {
	Authenticated bool     `json:"authenticated"`
	User          userInfo `json:"user"`
}

type WebhookHandler struct {
	Authenticator authenticator.Token
	Authorizer    authorizer.Authorizer
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var data map[string]interface{}
	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()
	err := decoder.Decode(&data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var apiVersion = data["apiVersion"].(string)
	var kind = data["kind"].(string)

	if apiVersion != "authentication.k8s.io/v1beta1" && apiVersion != "authorization.k8s.io/v1beta1" {
		http.Error(w, fmt.Sprintf("unknown apiVersion %q", apiVersion),
			http.StatusBadRequest)
		return
	}
	if kind == "TokenReview" {
		var token = data["spec"].(map[string]interface{})["token"].(string)
		h.authenticateToken(w, r, token, data)
	} else if kind == "SubjectAccessReview" {
		h.authorizeToken(w, r, data)
	} else {
		http.Error(w, fmt.Sprintf("unknown kind/apiVersion %q %q", kind, apiVersion),
			http.StatusBadRequest)
	}
}

func (h *WebhookHandler) authenticateToken(w http.ResponseWriter, r *http.Request, token string, data map[string]interface{}) {
	user, authenticated, err := h.Authenticator.AuthenticateToken(token)

	if !authenticated {
		var response status
		response.Authenticated = false
		data["status"] = response

		output, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(output)
		return
	}

	var info userInfo
	info.Username = user.GetName()
	info.UID = user.GetUID()
	info.Groups = user.GetGroups()
	info.Extra = user.GetExtra()

	var response status
	response.Authenticated = true
	response.User = info

	data["status"] = response

	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(output)
}

func (h *WebhookHandler) authorizeToken(w http.ResponseWriter, r *http.Request, data map[string]interface{}) {
	fmt.Printf("authorizeToken data : %#v\n", data)
	temp, _ := json.MarshalIndent(data, "", "  ")
	fmt.Printf("REQUEST : %s\n", string(temp))

	_, _, err := h.Authorizer.Authorize(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	delete(data, "spec")
	data["status"] = map[string]interface{} {
		"allowed": true,
	}
	output, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(output)
}