ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))

OS := $(shell uname -s 2>/dev/null || echo Unknown)

ifeq ($(OS),$(filter $(OS),Windows_NT MINGW64_NT))
HELP_CMD = powershell -NoProfile -Command "Get-Content '$(ROOT_DIR)Makefile' | Select-String -Pattern '^[a-zA-Z_-]+:.*?\#\#' | ForEach-Object { $$_ -match '^([a-zA-Z_-]+):.*?\#\#\s*(.*)$$' | Out-Null; '{0,-25}{1}' -f $$matches[1],$$matches[2] }"
else
HELP_CMD = sed -n 's/^\([a-zA-Z_-]*\):.*\#\#/\1/p' $(ROOT_DIR)Makefile | awk -F'##' '{printf "%-25s %s\n", $$1, $$2}'
endif

.PHONY: help
help: ## display this help
	@$(HELP_CMD)
