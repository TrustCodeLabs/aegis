GO ?= go
COVERPROFILE ?= coverage.out

.PHONY: test coverage clean

test:
	$(GO) test ./...

coverage:
	$(GO) test ./... -coverprofile=$(COVERPROFILE)
	$(GO) tool cover -func=$(COVERPROFILE) | tail -n 1

clean:
	rm -f $(COVERPROFILE)
