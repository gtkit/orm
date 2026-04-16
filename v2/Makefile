.PHONY: tool check tag gittag

LINT_TARGETS ?= ./...

tool: ## Lint Go code with the installed golangci-lint
	@ echo "▶️  golangci-lint run (v2)"
	golangci-lint run $(LINT_TARGETS)
	gofmt -l -w .
	@ echo "✅ golangci-lint done"

## govulncheck 检查漏洞 go install golang.org/x/vuln/cmd/govulncheck@latest
check:
	govulncheck ./...

## ────────────────────────────────────────────────────────
## v2 发版: make tag
## 读取 version.go 中的版本号，自动 bump patch，打 tag 并推送。
##
## Go modules 规范 (subdirectory module):
##   模块路径 = github.com/gtkit/orm/v2
##   代码目录 = v2/
##   Tag 前缀 = v2/    （必须与子目录名一致）
##   完整 tag  = v2/v2.x.y
##
## 用户使用: go get github.com/gtkit/orm/v2@v2.x.y
## ────────────────────────────────────────────────────────
tag:
	@current=$$(grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' version.go | head -n1 | tr -d 'v'); \
	if [ -z "$$current" ]; then echo "❌ version not found in version.go"; exit 1; fi; \
	maj=$$(echo $$current | cut -d. -f1); \
	min=$$(echo $$current | cut -d. -f2); \
	patch=$$(echo $$current | cut -d. -f3); \
	newpatch=$$(expr $$patch + 1); \
	new="v$$maj.$$min.$$newpatch"; \
	taglabel="v2/$$new"; \
	printf "Bump: v%s → %s (tag: %s)\n" "$$current" "$$new" "$$taglabel"; \
	sed -E -i.bak 's/(const Version = ")([^"]+)(")/\1'"$$new"'\3/' version.go && rm -f version.go.bak; \
	cd .. && git add v2/version.go; \
	git commit -m "chore(release): $$taglabel"; \
	git push gtkit HEAD; \
	git tag -a "$$taglabel" -m "release $$taglabel"; \
	git push gtkit "$$taglabel"; \
	printf "✅ v2 released: %s\n" "$$taglabel"

## 查看最新 v2 tag
gittag:
	@cd .. && git tag --sort=-version:refname -l 'v2/*' | head -1
