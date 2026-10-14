package handlers

// Example endpoints: delete this file when building a real service. They show
// the pattern every endpoint follows: verified caller (middleware) → validate
// input → Admin API → JSON answer. They need the scopes
// read_products,write_products.

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

const listProductsQuery = `
query exampleListProducts($first: Int!) {
  products(first: $first, sortKey: UPDATED_AT, reverse: true) {
    nodes {
      id
      title
      handle
      status
    }
  }
}`

const createProductMutation = `
mutation exampleCreateProduct($product: ProductCreateInput!) {
  productCreate(product: $product) {
    product {
      id
      title
      handle
      status
    }
    userErrors {
      field
      message
    }
  }
}`

type product struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Handle string `json:"handle"`
	Status string `json:"status"`
}

// ListProducts handles GET /example/products?first=5 (read_products).
type ListProducts struct {
	Admin  Admin
	Logger *slog.Logger
}

func (h ListProducts) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	first, err := strconv.Atoi(r.URL.Query().Get("first"))
	if err != nil || first < 1 || first > 50 {
		first = 5
	}
	var data struct {
		Products struct {
			Nodes []product `json:"nodes"`
		} `json:"products"`
	}
	if err := h.Admin.Do(r.Context(), listProductsQuery, map[string]any{"first": first}, &data); err != nil {
		failShopify(w, h.Logger, "[Example] product list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": data.Products.Nodes})
}

// CreateProduct handles POST /example/products with {"title": "..."}
// (write_products). Products are created as drafts, so a test never shows up
// in the storefront.
type CreateProduct struct {
	Admin  Admin
	Logger *slog.Logger
}

func (h CreateProduct) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	title := strings.TrimSpace(body.Title)
	if title == "" || len(title) > 255 {
		writeError(w, http.StatusBadRequest, "title must be 1 to 255 characters")
		return
	}

	var data struct {
		ProductCreate struct {
			Product    *product `json:"product"`
			UserErrors []struct {
				Field   []string `json:"field"`
				Message string   `json:"message"`
			} `json:"userErrors"`
		} `json:"productCreate"`
	}
	vars := map[string]any{"product": map[string]any{"title": title, "status": "DRAFT"}}
	if err := h.Admin.Do(r.Context(), createProductMutation, vars, &data); err != nil {
		failShopify(w, h.Logger, "[Example] product create failed", err)
		return
	}
	if len(data.ProductCreate.UserErrors) > 0 || data.ProductCreate.Product == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "Shopify rejected the product", "details": data.ProductCreate.UserErrors})
		return
	}
	h.Logger.Info("[Example] Product created", "id", data.ProductCreate.Product.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"product": data.ProductCreate.Product})
}
