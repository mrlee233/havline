package backup

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/scrypt"
)

// 加密归档的容器格式：magic + salt + nonce + AES-256-GCM 密文。
//
// 为什么不用「拿 session_secret 直接加密」：那份密钥本来就在数据目录里，
// 跟着归档一起发出去等于没加密。用户填的口令才是唯一不随文件流动的东西。
var encMagic = []byte("HAVLINE-ENC1\n")

// ErrPassphraseRequired 表示这份归档被口令加密过，必须提供口令才能读取。
var ErrPassphraseRequired = errors.New("该备份已加密，需要提供口令")

// 加密参数：默认值在 NAS 这类低功耗设备上约需几十到一百毫秒，
// 对「一天一次备份」完全可接受，同时把离线爆破成本抬到足够高。
const (
	scryptN       = 1 << 15
	scryptR       = 8
	scryptP       = 1
	scryptKeyLen  = 32
	scryptSaltLen = 16
)

// IsEncrypted 判断一份归档是否被口令加密（只读开头几个字节）。
func IsEncrypted(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()
	head := make([]byte, len(encMagic))
	if _, err := io.ReadFull(file, head); err != nil {
		// 短到连 magic 都放不下：只能是空文件或残件，交给后续解压去报错
		return false, nil
	}
	return string(head) == string(encMagic), nil
}

// sealFile 把 src 全量读出，加密后写进 dst。
// 备份受「恢复上传 64MB」这一上限约束，整份读进内存是可控的，
// 换来的是不需要自己拼分块 GCM 的复杂度。
func sealFile(dst io.Writer, src io.Reader, passphrase string) error {
	if passphrase == "" {
		return fmt.Errorf("口令不能为空")
	}
	salt := make([]byte, scryptSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	plain, err := io.ReadAll(src)
	if err != nil {
		return err
	}

	// magic 作为附加认证数据：容器格式被改动（比如换成别的格式）时解密直接失败
	if _, err := dst.Write(encMagic); err != nil {
		return err
	}
	if _, err := dst.Write(salt); err != nil {
		return err
	}
	if _, err := dst.Write(nonce); err != nil {
		return err
	}
	_, err = dst.Write(gcm.Seal(nil, nonce, plain, encMagic))
	return err
}

// openFile 解开一份加密归档，返回可读的明文流。
func openSealedFile(src io.Reader, passphrase string) (io.Reader, error) {
	head := make([]byte, len(encMagic))
	if _, err := io.ReadFull(src, head); err != nil {
		return nil, fmt.Errorf("无效的备份文件")
	}
	if string(head) != string(encMagic) {
		return nil, fmt.Errorf("无效的备份文件")
	}
	salt := make([]byte, scryptSaltLen)
	if _, err := io.ReadFull(src, salt); err != nil {
		return nil, fmt.Errorf("备份文件已损坏")
	}
	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(src, nonce); err != nil {
		return nil, fmt.Errorf("备份文件已损坏")
	}
	ciphertext, err := io.ReadAll(src)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, encMagic)
	if err != nil {
		// 校验失败只说明「密钥不对或内容被改过」，对用户是同一件事
		return nil, fmt.Errorf("口令错误，或备份文件已被修改")
	}
	return newByteReader(plain), nil
}

// newByteReader 把内存里的明文包成 io.Reader；抽出来是为了让调用点读起来短一点。
func newByteReader(data []byte) io.Reader {
	return bytes.NewReader(data)
}

func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	key, err := scrypt.Key([]byte(passphrase), salt, scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return nil, fmt.Errorf("派生加密密钥失败: %w", err)
	}
	return key, nil
}
