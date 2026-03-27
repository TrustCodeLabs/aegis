package localstorage_test

import (
	"context"
	"strings"
	"testing"

	"aegis/drivers/localstorage"
)

func TestDriverRejectsEscapingPaths(t *testing.T) {
	driver, err := localstorage.New(t.TempDir())
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}

	err = driver.Write(context.Background(), "../secret.txt", []byte("nope"))
	if err == nil {
		t.Fatalf("expected path escape to fail")
	}
	if !strings.Contains(err.Error(), "escapes storage root") {
		t.Fatalf("unexpected error: %v", err)
	}
}
