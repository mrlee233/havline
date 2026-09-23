package agent

import (
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"strings"
)

// apr1Itoa64 是 apr1（Apache MD5 crypt）使用的自定义 base64 字母表，同时用作随机盐字符集。
const apr1Itoa64 = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// apr1Crypt 生成 apr1 哈希，与 `openssl passwd -apr1` 等价（纯 Go，无外部命令依赖）。
// 选 apr1 而非 bcrypt：Nginx 对 $apr1$ 是自带实现，跨发行版稳定；bcrypt 依赖系统 crypt(3)，
// 在 CentOS 7 这类老 glibc 上无法校验。
func apr1Crypt(password string) (string, error) {
	salt, err := apr1RandomSalt(8)
	if err != nil {
		return "", err
	}
	return apr1CryptWithSalt(password, salt), nil
}

// apr1RandomSalt 生成指定长度的随机盐（字符取自 itoa64，与 openssl 行为一致）。
func apr1RandomSalt(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("生成随机盐失败: %w", err)
	}
	var b strings.Builder
	for _, v := range buf {
		b.WriteByte(apr1Itoa64[int(v)%len(apr1Itoa64)])
	}
	return b.String(), nil
}

// apr1CryptWithSalt 是 apr1 的确定性实现（salt 取前 8 字符），供单元测试用参考向量比对。
// 算法来自 Apache apr_md5_encode：MD5 三轮 + 1000 次迭代 + 自定义字节序 base64。
func apr1CryptWithSalt(password, salt string) string {
	if len(salt) > 8 {
		salt = salt[:8]
	}
	ctx := md5.New()
	ctx.Write([]byte(password))
	ctx.Write([]byte("$apr1$"))
	ctx.Write([]byte(salt))

	alt := md5.Sum([]byte(password + salt + password))
	for i := len(password); i > 0; i -= 16 {
		n := i
		if n > 16 {
			n = 16
		}
		ctx.Write(alt[:n])
	}
	for i := len(password); i > 0; i >>= 1 {
		if i&1 != 0 {
			ctx.Write([]byte{0})
		} else {
			ctx.Write([]byte(password[0:1]))
		}
	}
	final := ctx.Sum(nil)

	for i := 0; i < 1000; i++ {
		round := md5.New()
		if i&1 != 0 {
			round.Write([]byte(password))
		} else {
			round.Write(final)
		}
		if i%3 != 0 {
			round.Write([]byte(salt))
		}
		if i%7 != 0 {
			round.Write([]byte(password))
		}
		if i&1 != 0 {
			round.Write(final)
		} else {
			round.Write([]byte(password))
		}
		final = round.Sum(nil)
	}

	var out strings.Builder
	to64 := func(v uint32, n int) {
		for ; n > 0; n-- {
			out.WriteByte(apr1Itoa64[v&0x3f])
			v >>= 6
		}
	}
	to64(uint32(final[0])<<16|uint32(final[6])<<8|uint32(final[12]), 4)
	to64(uint32(final[1])<<16|uint32(final[7])<<8|uint32(final[13]), 4)
	to64(uint32(final[2])<<16|uint32(final[8])<<8|uint32(final[14]), 4)
	to64(uint32(final[3])<<16|uint32(final[9])<<8|uint32(final[15]), 4)
	to64(uint32(final[4])<<16|uint32(final[10])<<8|uint32(final[5]), 4)
	to64(uint32(final[11]), 2)

	return "$apr1$" + salt + "$" + out.String()
}
