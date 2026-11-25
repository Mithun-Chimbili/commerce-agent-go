package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const baseURL = "http://127.0.0.1:8000"

type Product struct {
	ID             string   `json:"id"`
	Category       string   `json:"category"`
	Name           string   `json:"name"`
	Price          int      `json:"price"`
	Fit            string   `json:"fit"`
	Occasion       string   `json:"occasion"`
	Rating         float64  `json:"rating"`
	AvailableSizes []string `json:"available_sizes"`
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

var reader = bufio.NewReader(os.Stdin)

func prompt(msg string) string {
	fmt.Print(msg)
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func apiCall(method, endpoint string, payload any) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewBuffer(b)
	}
	req, err := http.NewRequest(method, baseURL+endpoint, body)
	if err != nil {
		return 0, nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("\n[ERROR] Cannot connect to Dummy API Server. Run 'go run ./cmd/server' first.")
		return 503, nil, err
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, respBytes, nil
}

func runShoppingFlow() {
	query := prompt("\n[You] What are you looking for? (e.g., 'black cotton shirt under 1500'): ")
	if query == "" {
		return
	}

	fmt.Println("\n[Agent] Calling Intent Parser API...")
	status, data, err := apiCall("POST", "/api/agent/parse-intent", map[string]string{"query": query})
	if err != nil || status != 200 {
		return
	}
	var intent map[string]any
	_ = json.Unmarshal(data, &intent)
	fmt.Printf("[Agent] Extracted -> Category: %v | Max Price: ₹%v | Interests: %v\n",
		intent["category"], intent["max_price"], intent["keywords"])

	fmt.Println("[Agent] Calling Product Search & Ranking API across 300 items...")
	status, data, err = apiCall("POST", "/api/products/search", intent)
	if err != nil || status != 200 {
		return
	}

	var searchRes struct {
		Top3 []RankedProduct `json:"top_3"`
	}
	_ = json.Unmarshal(data, &searchRes)
	if len(searchRes.Top3) == 0 {
		fmt.Println("[Agent] No matching products found.")
		return
	}

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("TOP 3 RECOMMENDED PRODUCTS")
	fmt.Println(strings.Repeat("=", 70))
	for i, entry := range searchRes.Top3 {
		p := entry.Product
		fmt.Printf("%d. [%s] %s\n", i+1, p.ID, p.Name)
		fmt.Printf("   Price: ₹%d | Fit: %s | Occasion: %s | Rating: %.1f★\n", p.Price, p.Fit, p.Occasion, p.Rating)
		fmt.Printf("   Sizes: %s | Match Score: %.2f\n", strings.Join(p.AvailableSizes, ", "), entry.Score)
		fmt.Printf("   Why: %s\n", strings.Join(entry.MatchReasons, " ; "))
		fmt.Println(strings.Repeat("-", 70))
	}

	choiceStr := prompt("\n[Agent] Approve a product to checkout (Enter 1, 2, 3, or 0 to cancel): ")
	idx, err := strconv.Atoi(choiceStr)
	if err != nil || idx < 1 || idx > len(searchRes.Top3) {
		fmt.Println("[Agent] Checkout cancelled.")
		return
	}
	selected := searchRes.Top3[idx-1].Product
	fmt.Printf("\n[Agent] Product approved: %s (₹%d)\n", selected.Name, selected.Price)

	fmt.Println("[Agent] Initiating checkout via Dummy Razorpay Orders API...")
	status, data, err = apiCall("POST", "/api/razorpay/orders", map[string]int{"amount": selected.Price})
	if err != nil || status != 200 {
		fmt.Println("[Agent] Failed to create Razorpay order.")
		return
	}
	var rzpOrder map[string]any
	_ = json.Unmarshal(data, &rzpOrder)
	rzpOrderID := rzpOrder["id"].(string)
	fmt.Printf("[Agent] Razorpay Order Created -> ID: %s | Amount: ₹%d.00\n", rzpOrderID, selected.Price)

	methods := map[string]string{
		"1": "UPI",
		"2": "Credit/Debit Card",
		"3": "NetBanking",
		"4": "Digital Wallet",
	}

	maxAttempts := 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		fmt.Printf("\n--- Payment Attempt %d of %d ---\n", attempt, maxAttempts)
		fmt.Println("Select Payment Method:")
		fmt.Println("  1. UPI")
		fmt.Println("  2. Credit/Debit Card")
		fmt.Println("  3. NetBanking")
		fmt.Println("  4. Digital Wallet")

		pmChoice := prompt("[Agent] Choose payment method (1-4, or 0 to cancel): ")
		if pmChoice == "0" {
			fmt.Println("[Agent] Checkout cancelled.")
			return
		}
		chosenMethod, ok := methods[pmChoice]
		if !ok {
			fmt.Println("[Agent] Invalid payment method. Try again.")
			continue
		}

		paymentDetails := ""
		paymentMeta := map[string]string{}

		switch chosenMethod {
		case "Credit/Debit Card":
			cardNum := strings.ReplaceAll(prompt("  Enter Dummy 16-digit Card Number (end in 0000 to test decline): "), " ", "")
			expiry := prompt("  Enter Expiry (MM/YY, e.g., 12/29; use 01/20 to test expired): ")
			cvv := prompt("  Enter 3-digit CVV (e.g., 123; use 000 to test invalid CVV): ")
			if len(cardNum) < 4 {
				fmt.Println("[Agent] Invalid card number.")
				continue
			}
			paymentDetails = fmt.Sprintf("Card ending in %s (Exp: %s)", cardNum[len(cardNum)-4:], expiry)
			paymentMeta = map[string]string{"card_num": cardNum, "expiry": expiry, "cvv": cvv}

		case "UPI":
			upiID := prompt("  Enter Dummy UPI ID (e.g., success@razorpay; use fail@razorpay to test decline): ")
			paymentDetails = "UPI ID: " + upiID
			paymentMeta = map[string]string{"upi_id": upiID}

		case "NetBanking":
			bank := prompt("  Enter Bank Name (e.g., HDFC, SBI, ICICI): ")
			paymentDetails = "Bank: " + bank

		case "Digital Wallet":
			wallet := prompt("  Enter Wallet Name (e.g., Paytm, PhonePe, AmazonPay): ")
			paymentDetails = "Wallet: " + wallet
		}

		confirm := strings.ToLower(prompt(fmt.Sprintf("[Agent] Confirm payment of ₹%d via %s (%s)? (yes/no): ", selected.Price, chosenMethod, paymentDetails)))
		if confirm != "yes" && confirm != "y" {
			fmt.Println("[Agent] Payment cancelled by user.")
			return
		}

		fmt.Printf("\n[Agent] Processing dummy Razorpay payment via %s...\n", chosenMethod)
		status, data, err = apiCall("POST", "/api/razorpay/pay", map[string]any{
			"razorpay_order_id": rzpOrderID,
			"payment_method":    chosenMethod,
			"payment_details":   paymentDetails,
			"payment_meta":      paymentMeta,
			"product":           selected,
		})
		if err != nil {
			return
		}

		if status == 200 {
			var res struct {
				Order OrderRecord `json:"order"`
			}
			_ = json.Unmarshal(data, &res)
			o := res.Order
			fmt.Println("\n" + strings.Repeat("=", 70))
			fmt.Println("ORDER CONFIRMED & SAVED TO SQLITE3")
			fmt.Println(strings.Repeat("=", 70))
			fmt.Printf("Order ID            : %s  <-- Save this to retrieve your order\n", o.OrderID)
			fmt.Printf("Razorpay Order ID   : %s\n", o.RazorpayOrderID)
			fmt.Printf("Razorpay Payment ID : %s\n", o.RazorpayPaymentID)
			fmt.Printf("Product             : %s (%s)\n", o.ProductName, o.ProductID)
			fmt.Printf("Amount Paid         : ₹%d\n", o.Amount)
			fmt.Printf("Payment Method      : %s\n", o.PaymentMethod)
			fmt.Printf("Payment Details     : %s\n", o.PaymentDetails)
			fmt.Printf("Status              : %s\n", o.Status)
			fmt.Printf("Timestamp           : %s\n", o.CreatedAt)
			fmt.Println(strings.Repeat("=", 70))
			return
		}

		var errRes struct {
			Error map[string]string `json:"error"`
		}
		_ = json.Unmarshal(data, &errRes)
		fmt.Printf("\n[PAYMENT FAILED] %s: %s\n", errRes.Error["code"], errRes.Error["description"])

		if attempt < maxAttempts {
			retry := strings.ToLower(prompt("[Agent] Retry with another card or payment method? (yes/no): "))
			if retry != "yes" && retry != "y" {
				fmt.Println("[Agent] Checkout aborted.")
				return
			}
		} else {
			fmt.Println("[Agent] Maximum payment attempts reached. Order was not placed.")
		}
	}
}

func retrieveOrderFlow() {
	orderID := prompt("\n[You] Enter your Order ID (e.g., ORD-20261124-XXXXXX): ")
	if orderID == "" {
		return
	}

	fmt.Printf("[Agent] Calling Order Retrieval API for '%s'...\n", orderID)
	status, data, err := apiCall("GET", "/api/orders/"+orderID, nil)
	if err != nil {
		return
	}

	if status == 200 {
		var res struct {
			Order OrderRecord `json:"order"`
		}
		_ = json.Unmarshal(data, &res)
		o := res.Order
		fmt.Println("\n" + strings.Repeat("-", 55))
		fmt.Println("ORDER DETAILS RETRIEVED FROM SQLITE3")
		fmt.Println(strings.Repeat("-", 55))
		fmt.Printf("Order ID            : %s\n", o.OrderID)
		fmt.Printf("Razorpay Order ID   : %s\n", o.RazorpayOrderID)
		fmt.Printf("Razorpay Payment ID : %s\n", o.RazorpayPaymentID)
		fmt.Printf("Product             : %s (%s)\n", o.ProductName, o.ProductID)
		fmt.Printf("Amount Paid         : ₹%d\n", o.Amount)
		fmt.Printf("Payment Method      : %s\n", o.PaymentMethod)
		fmt.Printf("Payment Details     : %s\n", o.PaymentDetails)
		fmt.Printf("Status              : %s\n", o.Status)
		fmt.Printf("Created At          : %s\n", o.CreatedAt)
		fmt.Println(strings.Repeat("-", 55))
	} else {
		fmt.Printf("[Agent] Order '%s' not found in SQLite3.\n", orderID)
	}
}

func main() {
	fmt.Println(strings.Repeat("=", 70))
	fmt.Println("AGENTIC COMMERCE SYSTEM (GO | API-ONLY | DUMMY RAZORPAY | SQLITE3)")
	fmt.Println(strings.Repeat("=", 70))

	for {
		fmt.Println("\nMain Menu:")
		fmt.Println("  1. Shop with Commerce Agent")
		fmt.Println("  2. Retrieve Order by Order ID")
		fmt.Println("  3. Exit")

		switch prompt("Select an option (1-3): ") {
		case "1":
			runShoppingFlow()
		case "2":
			retrieveOrderFlow()
		case "3":
			fmt.Println("Goodbye!")
			return
		default:
			fmt.Println("Invalid choice. Enter 1, 2, or 3.")
		}
	}
}
