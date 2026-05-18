#!/usr/bin/env bash
# Printed by `make help` so Windows/PowerShell/cmd do not mangle quotes or `echo`/printf handling.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

# TODO: Revise this and format it so that it is consistent column-wise.
# TODO: rewrite the helper text to mention less about the commands and more about what is happening at a higher level.


echo "////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\"
echo
echo "                                         '||           ||           "
echo "  ... ...    ...   ... ..    ....    ....   ||   ....   ...  .. ...   "
echo "   ||'  || .|  '|.  ||' '' .|   '' .|...||  ||  '' .||   ||   ||  ||  "
echo "   ||    | ||   ||  ||     ||      ||       ||  .|' ||   ||   ||  ||  "
echo "   ||...'   '|..|' .||.     '|...'  '|...' .||. '|..'|' .||. .||. ||. "
echo "   ||                                                                 "
echo "  ''''                                                                "
echo
echo "  Porcelain - A modern workspace for AI"
echo
echo "\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////\\\\\\\\////"
echo
echo "  make up                               installs, builds, and runs [locus-desktop]"
echo
echo "  make install                          bootstrap dependencies for all products"
echo "  make build                            assemble all products into executables"
echo "  make configure                        materialize config for runtime state"
echo "  make run                              execute the [locus-desktop] application"
echo "  make test                             validate correctness and behavior"
echo "  make clean                            cleans workspace completely (requires CONFIRM=1)"
echo "  make clean-data                       remove Chimera runtime data (requires CONFIRM=1)"
echo
echo "  make release-install                  install GoReleaser + release hook deps"
echo "  make release-build                    cross-platform release archives (dist/)"
echo "  make release-package                  personal desktop bundle (dist/personal/)"
echo
echo " __                ___  __       ";
echo "/  \` |__| |  |\\/| |__  |__)  /\\  ";
echo "\\__, |  | |  |  | |___ |  \\ /~~\\ ";
echo "                                 ";
echo "            +------------+------------+-----------+------------+--------------+"
echo "  PRODUCTS: | broker     | gateway    | indexer   | supervisor | vectorstore  |"
echo "            +------------+------------+-----------+------------+--------------+"
echo
echo "  make chimera-<product>-install        bootstrap the product's pre-requisites"
echo "  make chimera-<product>-build          assemble the product into an executable"
echo "  make chimera-<product>-configure      materialize config for runtime state"
echo "  make chimera-<product>-run            execute the product"
echo "  make chimera-<product>-test           validate correctness and behavior"
echo "  make chimera-<product>-clean          purge generated files and clean workspace"
echo
echo "      __   __        __  ";
echo "|    /  \\ /  \` |  | /__\` ";
echo "|___ \\__/ \\__, \\__/ .__/ ";
echo "                         ";                                        
echo "            +------------------------------------------------------------------+"
echo "  PRODUCTS: | desktop                                                          |"
echo "            +------------------------------------------------------------------+"
echo
echo "  make locus-<product>-install          bootstrap the product's pre-requisites"
echo "  make locus-<product>-build            assemble the product into an executable"
echo "  make locus-<product>-run              execute the product"
echo "  make locus-<product>-test             validate correctness and behavior"
echo "  make locus-<product>-clean            purge generated files and clean workspace"
echo
echo "----/////----/////----/////----/////----/////----/////----/////----/////---/////---"
echo
echo "___  __   __        __  ";
echo " |  /  \\ /  \\ |    /__\` ";
echo " |  \\__/ \\__/ |___ .__/ ";
echo "                        ";
echo "  make fmt-check|fmt                    check code format changes and fixes it"
echo "  make vet                              validate code correctness and behavior"
echo "  make precommit                        prepare the code for a commit"
echo
echo "  make bash                             interactive bash (-il); Windows: Git  bash"
echo "  make tokencount-file                  FILE=path -> cl100k_base/o200k_base counts"
echo "  make catalog-fetch-free               fetch free tier models from pricing docs on web"
echo "  make catalog-fetch-available          fetch available models from chimera-broker"
echo "  make catalog-calculate                intersection of free and available models"
echo
