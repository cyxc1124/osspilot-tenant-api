package bucket

import "testing"

func TestValidateCorsRules(t *testing.T) {
	age := 3600
	got, err := validateCorsRules([]corsRule{{
		AllowedOrigins: []string{"https://console.example.com"},
		AllowedMethods: []string{"get", "PUT"},
		AllowedHeaders: []string{"*"},
		ExposeHeaders:  []string{"ETag"},
		MaxAgeSeconds:  &age,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].AllowedMethods[0] != "GET" || got[0].AllowedMethods[1] != "PUT" {
		t.Fatalf("methods %#v", got[0].AllowedMethods)
	}
	if _, err := validateCorsRules([]corsRule{{
		AllowedOrigins: []string{"not-a-url"},
		AllowedMethods: []string{"GET"},
	}}); err == nil {
		t.Fatal("want origin error")
	}
	if _, err := validateCorsRules(nil); err == nil {
		t.Fatal("want empty error")
	}
	def := defaultCorsRules([]string{"https://console.example.com"})
	if len(def) != 1 || def[0].AllowedMethods[0] != "GET" {
		t.Fatalf("default %#v", def)
	}
}
