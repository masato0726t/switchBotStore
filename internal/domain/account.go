// Package domain は SwitchBot データ収集のドメインモデルを定義する。
//
// このパッケージは標準ライブラリ以外に依存しない。他の内部パッケージも
// import してはならない（クリーンアーキテクチャの最内層）。
package domain

// AccountID は永続化された API アカウントの識別子（api_accounts.id）。
type AccountID int64

// Credential は SwitchBot API の認証情報。
type Credential struct {
	Token  string
	Secret string
}

// Account は SwitchBot API のアカウント。
type Account struct {
	Name       string
	Credential Credential
}
