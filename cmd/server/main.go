package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	mrand "math/rand"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	productsFile = "products.json"
	dbFile       = "commerce.db"
	serverAddr   = "127.0.0.1:8000"
)

type Product struct {
	ID             string   `json:"id"`
	Category       string   `json:"category"`
	Name           string   `json:"name"`
	Brand          string   `json:"brand"`
	Type           string   `json:"type"`
	Color          string   `json:"color"`
	Material       string   `json:"material"`
	Fit            string   `json:"fit"`
	Occasion       string   `json:"occasion"`
	AvailableSizes []string `json:"available_sizes"`
	Price          int      `json:"price"`
	Rating         float64  `json:"rating"`
	Stock          int      `json:"stock"`
}

type IntentResponse struct {
	Category string   `json:"category"`
	MaxPrice int      `json:"max_price"`
	Keywords []string `json:"keywords"`
}

type RankedProduct struct {
	Product      Product  `json:"product"`
	Score        float64  `json:"score"`
	MatchReasons []string `json:"match_reasons"`
}

type OrderRecord struct {
	OrderID           string `json:"order_id"`
	RazorpayOrderID   string `json:"razorpay_order_id"`
	RazorpayPaymentID string `json:"razorpay_payment_id"`
	ProductID         string `json:"product_id"`
	ProductName       string `json:"product_name"`
	Amount            int    `json:"amount"`
	PaymentMethod     string `json:"payment_method"`
	PaymentDetails    string `json:"payment_details"`
	Status            string `json:"status"`
	CreatedAt         string `json:"created_at"`
}

var db *sql.DB

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func ensureDataset() error {
	if _, err := os.Stat(productsFile); err == nil {
		return nil
	}

	rng := mrand.New(mrand.NewSource(42))
	brands := []string{"UrbanCraft", "NordicWear", "Veloce", "ApexApparel", "Solstice", "CrestLine", "Monolith", "Kova"}
	colors := []string{"Black", "White", "Navy", "Olive", "Beige", "Grey", "Maroon", "Blue", "Brown", "Charcoal"}
	sizes := []string{"S", "M", "L", "XL", "XXL"}
	occasions := []string{"Casual", "Formal", "Office", "Party", "Outdoor", "Travel"}

	type catSpec struct {
		category  string
		prefix    string
		types     []string
		materials []string
		fits      []string
		minPrice  int
		maxPrice  int
	}

	specs := []catSpec{
		{
			category:  "shirt",
			prefix:    "SHIRT",
			types:     []string{"Oxford Shirt", "Polo Shirt", "Linen Shirt", "Flannel Shirt", "Dress Shirt", "Henley Shirt"},
			materials: []string{"Cotton", "Linen", "Poly-Cotton", "Flannel", "Bamboo Blend", "Poplin"},
			fits:      []string{"Slim Fit", "Regular Fit", "Relaxed Fit", "Tailored Fit"},
			minPrice:  699, maxPrice: 3499,
		},
		{
			category:  "pant",
			prefix:    "PANT",
			types:     []string{"Chinos", "Slim Jeans", "Cargo Pants", "Formal Trousers", "Joggers", "Corduroy Pants"},
			materials: []string{"Denim", "Cotton Twill", "Linen Blend", "Poly-Viscose", "Stretch Cotton", "Corduroy"},
			fits:      []string{"Slim Fit", "Straight Fit", "Tapered Fit", "Relaxed Fit"},
			minPrice:  999, maxPrice: 4499,
		},
		{
			category:  "jacket",
			prefix:    "JKT",
			types:     []string{"Bomber Jacket", "Leather Jacket", "Denim Jacket", "Puffer Jacket", "Windbreaker", "Blazer Jacket"},
			materials: []string{"Faux Leather", "Denim", "Nylon", "Polyester Fleece", "Wool Blend", "Waterproof Shell"},
			fits:      []string{"Regular Fit", "Boxy Fit", "Slim Fit", "Oversized Fit"},
			minPrice:  1999, maxPrice: 8999,
		},
	}

	var products []Product
	for _, s := range specs {
		for i := 1; i <= 100; i++ {
			brand := brands[rng.Intn(len(brands))]
			color := colors[rng.Intn(len(colors))]
			mat := s.materials[rng.Intn(len(s.materials))]
			itemType := s.types[rng.Intn(len(s.types))]
			fit := s.fits[rng.Intn(len(s.fits))]
			occ := occasions[rng.Intn(len(occasions))]

			steps := (s.maxPrice - s.minPrice) / 50
			price := s.minPrice + rng.Intn(steps+1)*50
			rating := math.Round((3.5+rng.Float64()*1.5)*10) / 10

			perm := rng.Perm(len(sizes))
			count := 3 + rng.Intn(3)
			var avail []string
			for _, idx := range perm[:count] {
				avail = append(avail, sizes[idx])
			}

			products = append(products, Product{
				ID:             fmt.Sprintf("%s-%03d", s.prefix, i),
				Category:       s.category,
				Name:           fmt.Sprintf("%s %s %s %s", brand, color, mat, itemType),
				Brand:          brand,
				Type:           itemType,
				Color:          color,
				Material:       mat,
				Fit:            fit,
				Occasion:       occ,
				AvailableSizes: avail,
				Price:          price,
				Rating:         rating,
				Stock:          5 + rng.Intn(46),
			})
		}
	}

	data, err := json.MarshalIndent(products, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(productsFile, data, 0644); err != nil {
		return err
	}
	log.Printf("Generated %d items in %s", len(products), productsFile)
	return nil
}

func loadProducts() ([]Product, error) {
	data, err := os.ReadFile(productsFile)
	if err != nil {
		return nil, err
	}
	var products []Product
	err = json.Unmarshal(data, &products)
	return products, err
}

func initDB() error {
	var err error
	db, err = sql.Open("sqlite", dbFile)
	if err != nil {
		return err
	}
	schema := `
	CREATE TABLE IF NOT EXISTS orders (
		order_id TEXT PRIMARY KEY,
		razorpay_order_id TEXT NOT NULL,
		razorpay_payment_id TEXT NOT NULL,
		product_id TEXT NOT NULL,
		product_name TEXT NOT NULL,
		amount INTEGER NOT NULL,
		payment_method TEXT NOT NULL,
		payment_details TEXT NOT NULL,
		status TEXT NOT NULL,
		created_at TEXT NOT NULL
	);`
	_, err = db.Exec(schema)
	return err
}

func saveOrder(o OrderRecord) error {
	_, err := db.Exec(`
		INSERT INTO orders (
			order_id, razorpay_order_id, razorpay_payment_id,
			product_id, product_name, amount, payment_method,
			payment_details, status, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.OrderID, o.RazorpayOrderID, o.RazorpayPaymentID,
		o.ProductID, o.ProductName, o.Amount, o.PaymentMethod,
		o.PaymentDetails, o.Status, o.CreatedAt,
	)
	return err
}

func getOrderByID(orderID string) (*OrderRecord, error) {
	row := db.QueryRow(`
		SELECT order_id, razorpay_order_id, razorpay_payment_id,
		       product_id, product_name, amount, payment_method,
		       payment_details, status, created_at
		FROM orders WHERE order_id = ?`, orderID)

	var o OrderRecord
	err := row.Scan(
		&o.OrderID, &o.RazorpayOrderID, &o.RazorpayPaymentID,
		&o.ProductID, &o.ProductName, &o.Amount, &o.PaymentMethod,
		&o.PaymentDetails, &o.Status, &o.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func handleParseIntent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	q := strings.ToLower(req.Query)

	category := ""
	switch {
	case containsAny(q, []string{"shirt", "shirts", "polo", "flannel", "oxford"}):
		category = "shirt"
	case containsAny(q, []string{"pant", "pants", "trouser", "trousers", "jeans", "chino", "chinos", "jogger", "cargo"}):
		category = "pant"
	case containsAny(q, []string{"jacket", "jackets", "coat", "bomber", "blazer", "puffer", "windbreaker"}):
		category = "jacket"
	}

	maxPrice := 0
	priceRe := regexp.MustCompile(`(?:under|below|less than|max|budget|upto|up to|rs\.?|inr|₹)?\s*(\d{3,5})`)
	if m := priceRe.FindStringSubmatch(q); len(m) > 1 {
		maxPrice, _ = strconv.Atoi(m[1])
	}

	stopWords := map[string]bool{
		"want": true, "need": true, "looking": true, "for": true, "the": true, "under": true,
		"below": true, "less": true, "than": true, "with": true, "buy": true, "show": true,
		"best": true, "around": true, "budget": true, "max": true, "price": true, "inr": true,
		"shirt": true, "shirts": true, "pant": true, "pants": true, "jacket": true, "jackets": true,
		"find": true, "search": true, "get": true,
	}

	wordRe := regexp.MustCompile(`[a-z]+`)
	var keywords []string
	for _, tok := range wordRe.FindAllString(q, -1) {
		if len(tok) > 2 && !stopWords[tok] {
			keywords = append(keywords, tok)
		}
	}

	log.Printf("[API] POST /api/agent/parse-intent -> cat=%s maxPrice=%d kw=%v", category, maxPrice, keywords)
	writeJSON(w, http.StatusOK, IntentResponse{
		Category: category,
		MaxPrice: maxPrice,
		Keywords: keywords,
	})
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func handleSearchProducts(w http.ResponseWriter, r *http.Request) {
	var intent IntentResponse
	_ = json.NewDecoder(r.Body).Decode(&intent)

	products, err := loadProducts()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load catalog"})
		return
	}

	var scored []RankedProduct
	for _, item := range products {
		if intent.Category != "" && item.Category != intent.Category {
			continue
		}

		score := 0.0
		var reasons []string

		if intent.MaxPrice > 0 {
			if item.Price <= intent.MaxPrice {
				score += 40
				closeness := 1.0 - (float64(intent.MaxPrice-item.Price) / float64(intent.MaxPrice))
				score += closeness * 10
				reasons = append(reasons, fmt.Sprintf("Within budget (₹%d <= ₹%d)", item.Price, intent.MaxPrice))
			} else {
				overRatio := float64(item.Price-intent.MaxPrice) / float64(intent.MaxPrice)
				score -= (30 + overRatio*50)
			}
		} else {
			score += 20
		}

		searchable := strings.ToLower(strings.Join([]string{
			item.Name, item.Brand, item.Type, item.Color, item.Material, item.Fit, item.Occasion,
		}, " "))

		var matched []string
		for _, kw := range intent.Keywords {
			if strings.Contains(searchable, kw) {
				score += 25
				matched = append(matched, kw)
			}
		}
		if len(matched) > 0 {
			reasons = append(reasons, "Matches preferences: "+strings.Join(matched, ", "))
		}

		score += item.Rating * 4
		reasons = append(reasons, fmt.Sprintf("Rated %.1f/5.0", item.Rating))

		scored = append(scored, RankedProduct{
			Product:      item,
			Score:        math.Round(score*100) / 100,
			MatchReasons: reasons,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if len(scored) > 3 {
		scored = scored[:3]
	}

	log.Printf("[API] POST /api/products/search -> returned %d items", len(scored))
	writeJSON(w, http.StatusOK, map[string]any{"top_3": scored})
}

func handleRazorpayCreateOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Amount int `json:"amount"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	rzpOrderID := "order_Mock" + randomHex(5)
	log.Printf("[API] POST /api/razorpay/orders -> %s (₹%d)", rzpOrderID, req.Amount)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       rzpOrderID,
		"entity":   "order",
		"amount":   req.Amount * 100,
		"currency": "INR",
		"status":   "created",
	})
}

func handleRazorpayPay(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RazorpayOrderID string            `json:"razorpay_order_id"`
		PaymentMethod   string            `json:"payment_method"`
		PaymentDetails  string            `json:"payment_details"`
		PaymentMeta     map[string]string `json:"payment_meta"`
		Product         Product           `json:"product"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.PaymentMethod == "Credit/Debit Card" {
		cardNum := req.PaymentMeta["card_num"]
		cvv := req.PaymentMeta["cvv"]
		expiry := req.PaymentMeta["expiry"]

		if strings.HasSuffix(cardNum, "0000") {
			writeJSON(w, http.StatusPaymentRequired, map[string]any{
				"status": "failed",
				"error": map[string]string{
					"code":        "BAD_REQUEST_ERROR",
					"description": "Card declined by issuing bank (Insufficient funds).",
				},
			})
			return
		}
		if cvv == "000" || len(cvv) != 3 {
			writeJSON(w, http.StatusPaymentRequired, map[string]any{
				"status": "failed",
				"error": map[string]string{
					"code":        "GATEWAY_ERROR",
					"description": "Invalid CVV provided.",
				},
			})
			return
		}
		expRe := regexp.MustCompile(`^(0[1-9]|1[0-2])/\d{2}$`)
		if expiry == "01/20" || !expRe.MatchString(expiry) {
			writeJSON(w, http.StatusPaymentRequired, map[string]any{
				"status": "failed",
				"error": map[string]string{
					"code":        "BAD_REQUEST_ERROR",
					"description": "Card is expired or expiry format is invalid (use MM/YY).",
				},
			})
			return
		}
	} else if req.PaymentMethod == "UPI" {
		upi := strings.ToLower(req.PaymentMeta["upi_id"])
		if upi == "fail@razorpay" || !strings.Contains(upi, "@") {
			writeJSON(w, http.StatusPaymentRequired, map[string]any{
				"status": "failed",
				"error": map[string]string{
					"code":        "PAYMENT_DECLINED",
					"description": "UPI transaction declined or invalid VPA.",
				},
			})
			return
		}
	}

	order := OrderRecord{
		OrderID:           fmt.Sprintf("ORD-%s-%s", time.Now().Format("20060102"), randomHex(3)),
		RazorpayOrderID:   req.RazorpayOrderID,
		RazorpayPaymentID: "pay_Mock" + randomHex(5),
		ProductID:         req.Product.ID,
		ProductName:       req.Product.Name,
		Amount:            req.Product.Price,
		PaymentMethod:     req.PaymentMethod,
		PaymentDetails:    req.PaymentDetails,
		Status:            "PAID",
		CreatedAt:         time.Now().Format("2006-01-02 15:04:05"),
	}

	if err := saveOrder(order); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Database save failed"})
		return
	}

	log.Printf("[API] POST /api/razorpay/pay -> PAID & SAVED %s", order.OrderID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "order": order})
}

func handleGetOrder(w http.ResponseWriter, r *http.Request) {
	orderID := strings.TrimPrefix(r.URL.Path, "/api/orders/")
	order, err := getOrderByID(orderID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"status":  "not_found",
			"message": fmt.Sprintf("Order '%s' not found.", orderID),
		})
		return
	}
	log.Printf("[API] GET /api/orders/%s -> FOUND", orderID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "found", "order": order})
}

func main() {
	if err := ensureDataset(); err != nil {
		log.Fatalf("Dataset error: %v", err)
	}
	if err := initDB(); err != nil {
		log.Fatalf("SQLite3 error: %v", err)
	}
	defer db.Close()

	http.HandleFunc("/api/agent/parse-intent", handleParseIntent)
	http.HandleFunc("/api/products/search", handleSearchProducts)
	http.HandleFunc("/api/razorpay/orders", handleRazorpayCreateOrder)
	http.HandleFunc("/api/razorpay/pay", handleRazorpayPay)
	http.HandleFunc("/api/orders/", handleGetOrder)

	log.Printf("Dummy Commerce & Razorpay API Server running at http://%s", serverAddr)
	log.Fatal(http.ListenAndServe(serverAddr, nil))
}
