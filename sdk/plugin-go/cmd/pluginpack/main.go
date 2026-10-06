// Command pluginpack 用于打包并签名 AXmiPic 插件归档，便于发布到插件市场。
//
// 用法：
//
//	# 打包（可选同时签名）
//	pluginpack -dir plugins/smsbao -out smsbao-0.1.0.zip -key <base64-private-key>
//
//	# 仅签名已有归档
//	pluginpack -sign smsbao-0.1.0.zip -key <base64-private-key>
//
//	# 校验签名
//	pluginpack -verify smsbao-0.1.0.zip -pub <base64-public-key> -sig <base64-signature>
//
// 私钥支持 32 字节种子或 64 字节 Ed25519 私钥的 base64 编码。命令会输出归档的
// sha256 与（如签名）signature，可直接填入市场索引。
package main

import (
	"archive/zip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pluginpack:", err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("dir", "", "plugin directory to pack")
	out := flag.String("out", "", "output archive path")
	key := flag.String("key", "", "base64 ed25519 private key (pack/sign)")
	sign := flag.String("sign", "", "sign an existing archive instead of packing")
	verify := flag.String("verify", "", "verify an existing archive")
	pub := flag.String("pub", "", "base64 ed25519 public key (verify)")
	sig := flag.String("sig", "", "base64 signature (verify)")
	flag.Parse()

	switch {
	case *verify != "":
		return verifyArchive(*verify, *pub, *sig)
	case *sign != "":
		data, err := os.ReadFile(*sign)
		if err != nil {
			return err
		}
		return printArtifact(data, *key)
	default:
		if *dir == "" || *out == "" {
			return fmt.Errorf("-dir and -out are required for packing")
		}
		if err := pack(*dir, *out); err != nil {
			return err
		}
		data, err := os.ReadFile(*out)
		if err != nil {
			return err
		}
		fmt.Printf("archive=%s\n", *out)
		return printArtifact(data, *key)
	}
}

// pack 把目录打包为 zip，保留文件权限位。
func pack(dir, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to pack non-regular file %s", path)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		absOut, _ := filepath.Abs(out)
		absPath, _ := filepath.Abs(path)
		if absOut == absPath {
			return nil
		}
		hdr := &zip.FileHeader{Name: filepath.ToSlash(rel), Method: zip.Deflate}
		hdr.SetMode(info.Mode())
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = src.Close() }()
		_, err = io.Copy(w, src)
		return err
	})
	if err != nil {
		_ = zw.Close()
		return err
	}
	return zw.Close()
}

// printArtifact 输出 sha256，并在提供私钥时输出签名。
func printArtifact(data []byte, key string) error {
	sum := sha256.Sum256(data)
	fmt.Printf("sha256=%s\n", hex.EncodeToString(sum[:]))
	if strings.TrimSpace(key) == "" {
		return nil
	}
	priv, err := parsePrivateKey(key)
	if err != nil {
		return err
	}
	signature := ed25519.Sign(priv, data)
	fmt.Printf("signature=%s\n", base64.StdEncoding.EncodeToString(signature))
	return nil
}

// verifyArchive 校验归档签名。
func verifyArchive(path, pubB64, sigB64 string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	fmt.Printf("sha256=%s\n", hex.EncodeToString(sum[:]))
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil {
		return fmt.Errorf("invalid signature encoding")
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), data, sig) {
		return fmt.Errorf("signature verification failed")
	}
	fmt.Println("signature=OK")
	return nil
}

// parsePrivateKey 解析 base64 私钥（32 字节种子或 64 字节私钥）。
func parsePrivateKey(s string) (ed25519.PrivateKey, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("invalid private key encoding: %w", err)
	}
	switch len(b) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(b), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(b), nil
	default:
		return nil, fmt.Errorf("private key must be %d-byte seed or %d-byte key", ed25519.SeedSize, ed25519.PrivateKeySize)
	}
}
