package potoolcrypto

// 独立采购订单工具(po-tool)的密钥保护。
//
// 用一个共享口令把用友密钥加密成 blob。没有正确口令解不出密钥,
// 所以 exe 被拷走/拆开也调不了用友。口令既是"进门门槛", 也是"解密钥匙"。
//
// Argon2id(内存硬, 抗 GPU/ASIC 离线爆破)派生 key + AES-256-GCM, 纯 x/crypto。
//   - pack 工具:  Encrypt(用友密钥 json, 口令) → secret.dat
//   - po-tool 运行: Decrypt(secret.dat, 用户输入口令) → 用友密钥; 口令错则认证失败。

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id 参数(解锁一次约几十~几百ms, 用户无感)。
// 改这些参数会让旧 secret.dat 解不开, 须重新 pack。
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB = 64 MB
	argonThreads = 4
	saltLen      = 16
	keyLen       = 32 // AES-256
)

// utf8BOM Windows 编辑器另存可能在文件头加的字节顺序标记(用转义写, 避免源码里出现真 BOM)。
var utf8BOM = string([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM, 字节构造避免源码内出现真BOM

// ErrBadPassword 口令错误(GCM 认证失败)。
var ErrBadPassword = errors.New("口令错误, 无法解密密钥")

// deriveKey 从口令 + salt 用 Argon2id 派生 32 字节 AES-256 key。
func deriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, keyLen)
}

// Encrypt 用口令加密明文, 返回 base64(salt | nonce | ciphertext)。
func Encrypt(plaintext []byte, password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}
	gcm, err := newGCM(password, salt)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	blob := make([]byte, 0, len(salt)+len(nonce)+len(ct))
	blob = append(blob, salt...)
	blob = append(blob, nonce...)
	blob = append(blob, ct...)
	return base64.StdEncoding.EncodeToString(blob), nil
}

// Decrypt 用口令解密 base64 blob; 口令错 → GCM 认证失败 → ErrBadPassword。
func Decrypt(blobB64, password string) ([]byte, error) {
	// 容忍文件首尾空白/换行, 以及 Windows 编辑器可能加的 UTF-8 BOM
	clean := strings.TrimSpace(blobB64)
	clean = strings.TrimPrefix(clean, utf8BOM)
	clean = strings.TrimSpace(clean)
	blob, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return nil, errors.New("密钥文件损坏(非法 base64)")
	}
	if len(blob) < saltLen+12+16 { // salt + 最小 nonce(12) + GCM tag(16)
		return nil, errors.New("密钥文件损坏(长度不足)")
	}
	salt := blob[:saltLen]
	gcm, err := newGCM(password, salt)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(blob) < saltLen+ns+16 {
		return nil, errors.New("密钥文件损坏(长度不足)")
	}
	nonce := blob[saltLen : saltLen+ns]
	ct := blob[saltLen+ns:]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, ErrBadPassword
	}
	return plain, nil
}

func newGCM(password string, salt []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(deriveKey(password, salt))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
