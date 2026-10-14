package shopify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// maxThrottleRetries bounds how often a THROTTLED request is resent.
const maxThrottleRetries = 3

// Client sends Admin API GraphQL requests for one store.
type Client struct {
	shop       string
	apiVersion string
	tokens     *TokenStore
	httpClient *http.Client
}

// NewClient creates an Admin API client that uses the stored token.
func NewClient(shop, apiVersion string, tokens *TokenStore) *Client {
	return &Client{shop: shop, apiVersion: apiVersion, tokens: tokens, httpClient: &http.Client{Timeout: 30 * time.Second}}
}

// GraphQLError is one entry of a GraphQL `errors` array.
type GraphQLError struct {
	Message    string `json:"message"`
	Extensions struct {
		Code string `json:"code"`
	} `json:"extensions"`
}

// GraphQLErrors is returned when Shopify answers with top-level GraphQL errors.
type GraphQLErrors []GraphQLError

func (e GraphQLErrors) Error() string {
	if len(e) == 0 {
		return "graphql: unknown error"
	}
	return "graphql: " + e[0].Message
}

type graphqlResponse struct {
	Data       json.RawMessage `json:"data"`
	Errors     GraphQLErrors   `json:"errors"`
	Extensions struct {
		Cost struct {
			RequestedQueryCost float64 `json:"requestedQueryCost"`
			ThrottleStatus     struct {
				CurrentlyAvailable float64 `json:"currentlyAvailable"`
				RestoreRate        float64 `json:"restoreRate"`
			} `json:"throttleStatus"`
		} `json:"cost"`
	} `json:"extensions"`
}

var errUnauthorized = errors.New("shopify: unauthorized")

// Do runs a GraphQL query or mutation and decodes `data` into out.
//
// A 401 means the app was uninstalled (or the token revoked): the token is
// dropped and ErrNotConnected returned until the app is opened again. A
// THROTTLED answer (nothing was executed) waits for the bucket and retries.
func (c *Client) Do(ctx context.Context, query string, variables map[string]any, out any) error {
	for attempt := 0; ; attempt++ {
		res, err := c.send(ctx, query, variables)
		if errors.Is(err, errUnauthorized) {
			c.tokens.Forget()
			return ErrNotConnected
		}
		if err != nil {
			return err
		}
		if isThrottled(res.Errors) && attempt < maxThrottleRetries {
			if err := sleep(ctx, throttleWait(res)); err != nil {
				return err
			}
			continue
		}
		if len(res.Errors) > 0 {
			return res.Errors
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(res.Data, out)
	}
}

func (c *Client) send(ctx context.Context, query string, variables map[string]any) (*graphqlResponse, error) {
	token, err := c.tokens.Token(c.shop)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("https://%s/admin/api/%s/graphql.json", c.shop, c.apiVersion), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Shopify-Access-Token", token)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("admin api: %w", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode == http.StatusUnauthorized {
		return nil, errUnauthorized
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("admin api: Shopify answered %d", res.StatusCode)
	}
	var parsed graphqlResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("admin api: %w", err)
	}
	return &parsed, nil
}

func isThrottled(errs GraphQLErrors) bool {
	for _, e := range errs {
		if e.Extensions.Code == "THROTTLED" {
			return true
		}
	}
	return false
}

// throttleWait is how long until the bucket holds enough points for the request.
func throttleWait(res *graphqlResponse) time.Duration {
	cost := res.Extensions.Cost
	missing := cost.RequestedQueryCost - cost.ThrottleStatus.CurrentlyAvailable
	if cost.ThrottleStatus.RestoreRate <= 0 || missing <= 0 {
		return time.Second
	}
	return time.Duration(math.Ceil(missing/cost.ThrottleStatus.RestoreRate)) * time.Second
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
