package config

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/lynn/porcelain/internal/binfind"
	"github.com/lynn/porcelain/internal/naming"
)

// DefaultGatewayBin resolves chimera-gateway next to this executable or on PATH.
func DefaultGatewayBin() string {
	dir := executableDir()
	if p := binfind.FirstInExeDirs(dir, binfind.SearchNames(naming.ProductGatewayBinName)); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return naming.ProductGatewayBinName + ".exe"
	}
	return naming.ProductGatewayBinName
}

// DefaultBrokerBin resolves chimera-broker next to this executable or on PATH.
func DefaultBrokerBin() string {
	dir := executableDir()
	if p := binfind.FirstInExeDirs(dir, binfind.SearchNames(naming.ProductBrokerName)); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return naming.ProductBrokerName + ".exe"
	}
	return naming.ProductBrokerName
}

// DefaultVectorstoreBin resolves chimera-vectorstore next to this executable or on PATH.
func DefaultVectorstoreBin() string {
	dir := executableDir()
	return binfind.FirstInExeDirs(dir, binfind.SearchNames(naming.ProductVectorstoreName))
}

// DefaultQdrantBin resolves the Qdrant backend binary for chimera-vectorstore -bin.
func DefaultQdrantBin() string {
	dir := executableDir()
	names := []string{"qdrant"}
	if runtime.GOOS == "windows" {
		names = []string{"qdrant.exe", "qdrant"}
	}
	if p := binfind.FirstInExeDirs(dir, names); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return naming.ProductQdrantBinName + ".exe"
	}
	return naming.ProductQdrantBinName
}

// DefaultBifrostBin resolves the BiFrost HTTP backend binary for chimera-broker -bin.
func DefaultBifrostBin() string {
	dir := executableDir()
	names := []string{naming.ProductBifrostHTTPBinName, "bifrost"}
	if runtime.GOOS == "windows" {
		names = []string{naming.ProductBifrostHTTPBinName + ".exe", "bifrost.exe", naming.ProductBifrostHTTPBinName, "bifrost"}
	}
	if p := binfind.FirstInExeDirs(dir, names); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return naming.ProductBifrostHTTPBinName + ".exe"
	}
	return naming.ProductBifrostHTTPBinName
}

// DefaultIndexerBin resolves chimera-indexer next to this executable or on PATH.
func DefaultIndexerBin() string {
	dir := executableDir()
	if p := binfind.FirstInExeDirs(dir, binfind.SearchNames(naming.ProductIndexerBinName)); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return naming.ProductIndexerBinName + ".exe"
	}
	return naming.ProductIndexerBinName
}

func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}
