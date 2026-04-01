package hetzner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const baseURL = "https://api.hetzner.cloud/v1"

type Client struct {
	token      string
	httpClient *http.Client
}

type Server struct {
	ID        int               `json:"id"`
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	PublicNet struct {
		IPv4 struct {
			IP string `json:"ip"`
		} `json:"ipv4"`
	} `json:"public_net"`
	ServerType struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Cores       int     `json:"cores"`
		Memory      float64 `json:"memory"`
	} `json:"server_type"`
	Created time.Time         `json:"created"`
	Labels  map[string]string `json:"labels"`
}

type SSHKey struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) CreateServer(name, serverType, image, location, userData string, sshKeyIDs []int, labels map[string]string) (*Server, error) {
	body := map[string]interface{}{
		"name":        name,
		"server_type": serverType,
		"image":       image,
		"location":    location,
		"user_data":   userData,
		"ssh_keys":    sshKeyIDs,
		"labels":      labels,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	resp, err := c.do("POST", "/servers", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Server Server `json:"server"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("hetzner API error: %s: %s", result.Error.Code, result.Error.Message)
	}

	return &result.Server, nil
}

func (c *Client) GetServer(id int) (*Server, error) {
	resp, err := c.do("GET", fmt.Sprintf("/servers/%d", id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Server Server `json:"server"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &result.Server, nil
}

func (c *Client) DeleteServer(id int) error {
	resp, err := c.do("DELETE", fmt.Sprintf("/servers/%d", id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) ListServers(labelSelector string) ([]Server, error) {
	var all []Server
	page := 1

	for {
		url := fmt.Sprintf("/servers?label_selector=%s&page=%d&per_page=50", labelSelector, page)
		resp, err := c.do("GET", url, nil)
		if err != nil {
			return nil, err
		}

		var result struct {
			Servers []Server `json:"servers"`
			Meta    struct {
				Pagination struct {
					NextPage *int `json:"next_page"`
				} `json:"pagination"`
			} `json:"meta"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decoding response: %w", err)
		}
		resp.Body.Close()

		all = append(all, result.Servers...)

		if result.Meta.Pagination.NextPage == nil {
			break
		}
		page = *result.Meta.Pagination.NextPage
	}

	return all, nil
}

func (c *Client) ListSSHKeys() ([]SSHKey, error) {
	resp, err := c.do("GET", "/ssh_keys", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		SSHKeys []SSHKey `json:"ssh_keys"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return result.SSHKeys, nil
}

func (c *Client) WaitForRunning(id int, timeout time.Duration) (*Server, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		srv, err := c.GetServer(id)
		if err != nil {
			return nil, err
		}
		if srv.Status == "running" {
			fmt.Fprintln(os.Stderr)
			return srv, nil
		}
		fmt.Fprint(os.Stderr, ".")
		time.Sleep(2 * time.Second)
	}

	return nil, fmt.Errorf("timeout waiting for server %d to start", id)
}

func (c *Client) do(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)

		var apiErr struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Error.Message != "" {
			return nil, fmt.Errorf("hetzner API error (%d): %s: %s", resp.StatusCode, apiErr.Error.Code, apiErr.Error.Message)
		}
		return nil, fmt.Errorf("hetzner API error (%d): %s", resp.StatusCode, string(respBody))
	}

	return resp, nil
}
