package agent

import "testing"

// 参考向量由本机 OpenSSL 3.5.4 生成：
//
//	openssl passwd -apr1 -salt <salt> <password>
//
// 自研哈希必须与 openssl 完全一致，否则 Nginx Basic Auth 会永远认证失败。
func TestApr1CryptMatchesOpenSSLVectors(t *testing.T) {
	cases := []struct {
		password string
		salt     string
		want     string
	}{
		{"password", "xxxxxxxx", "$apr1$xxxxxxxx$dxHfLAsjHkDRmG83UXe8K0"},
		{"hello", "01234567", "$apr1$01234567$JGzOENmOOkvGYFfJQyG9X/"},
		{"P@ssw0rd!", "abcdefgh", "$apr1$abcdefgh$T9Ta7XuM2Yltpi/4uviFw0"},
	}
	for _, c := range cases {
		if got := apr1CryptWithSalt(c.password, c.salt); got != c.want {
			t.Errorf("apr1CryptWithSalt(%q, %q)\n got: %s\nwant: %s", c.password, c.salt, got, c.want)
		}
	}
}

func TestApr1CryptSaltHandling(t *testing.T) {
	// 超过 8 字符的盐只取前 8 位（与 openssl / apr 行为一致）
	long := apr1CryptWithSalt("password", "xxxxxxxxyyyy")
	short := apr1CryptWithSalt("password", "xxxxxxxx")
	if long != short {
		t.Errorf("盐应截断到 8 字符: %s vs %s", long, short)
	}
}

func TestApr1CryptRandomSalt(t *testing.T) {
	first, err := apr1Crypt("password")
	if err != nil {
		t.Fatalf("apr1Crypt 失败: %v", err)
	}
	second, err := apr1Crypt("password")
	if err != nil {
		t.Fatalf("apr1Crypt 失败: %v", err)
	}
	if first == second {
		t.Error("随机盐应产生不同哈希")
	}
	// 结构：$apr1$<8 位盐>$<22 位编码> = 37 字符
	if len(first) != 37 {
		t.Errorf("哈希长度应为 37，实际 %d: %s", len(first), first)
	}
	if first[:6] != "$apr1$" || first[14] != '$' {
		t.Errorf("哈希结构异常: %s", first)
	}
	if same := apr1CryptWithSalt("password", first[6:14]); same != first {
		t.Errorf("从哈希中取出的盐应能复现同一哈希: %s vs %s", same, first)
	}
}
