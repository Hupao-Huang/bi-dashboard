package potoolcrypto

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	plain := []byte(`{"appkey":"87d4f181","appsecret":"s3cr3t","base_url":"https://c3.yonyoucloud.com"}`)
	pwd := "测试口令"
	blob, err := Encrypt(plain, pwd)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := Decrypt(blob, pwd)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plain) {
		t.Fatalf("round-trip 不一致: got %q", got)
	}
}

func TestDecryptWrongPassword(t *testing.T) {
	blob, err := Encrypt([]byte("secret-data"), "测试口令")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(blob, "错误口令"); err != ErrBadPassword {
		t.Fatalf("错口令应返回 ErrBadPassword, got %v", err)
	}
}

func TestEncryptRandomizedSaltNonce(t *testing.T) {
	// 同明文同口令两次加密结果必须不同(salt/nonce 随机), 否则密文可被比对
	a, _ := Encrypt([]byte("x"), "pw")
	b, _ := Encrypt([]byte("x"), "pw")
	if a == b {
		t.Fatal("两次加密结果相同, salt/nonce 没随机")
	}
}

func TestDecryptGarbage(t *testing.T) {
	if _, err := Decrypt("not base64 !!!", "pw"); err == nil {
		t.Fatal("非法 base64 应报错")
	}
	if _, err := Decrypt("YWJj", "pw"); err == nil { // 合法 base64 但太短
		t.Fatal("过短 blob 应报错")
	}
}
