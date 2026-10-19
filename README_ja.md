# CAP Token Usage Tracker

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CLIProxyAPI](https://img.shields.io/badge/CLIProxyAPI-Plugin-success.svg)](https://github.com/router-for-me/CLIProxyAPI)

**[English](README.md)** | **[简体中文](README_zh.md)** | **[日本語](README_ja.md)** | **[Français](README_fr.md)** | **[한국어](README_ko.md)**

---

**CAP Token Usage Tracker** は、[CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) のための軽量・永続化トークン使用量＆コスト分析プラグインです。公式プラグイン機構と連携し、リクエストごとのメタデータと分単位の集計データをローカルの組み込みデータベースに安全に記録。直感的で高速なダッシュボードを提供します。

複雑な監視インフラを構築することなく、トークン消費構造、プロンプトキャッシュ（Prompt Cache）の効率、概算コスト、モデルごとの実測パフォーマンスをひと目で把握できます。

---

## 主な機能とダッシュボード画面

### 1. 全体トークン統計とキャッシュヒット率モニタリング

トラフィック規模とトークン内訳をリアルタイムに把握し、コンテキストキャッシュによるコスト削減効果を正確に測定します。

- **トークン構成の可視化**: 入力トークン、出力トークン、およびキャッシュヒットしたトークン数を直感的に色分け表示。
- **キャッシュ効率の測定**: プロンプトキャッシュのヒット率（Cache Hit Rate）を算出し、プロンプト再利用の効率を評価。
- **柔軟な時間軸集計**: 分・時間・日単位の切り替えに対応。データがまばらな区間もゼロ埋め処理され、常に正確なタイムラインを維持。

![トークン使用量とキャッシュ分析](assets/screenshots/01-token-overview.png)

---

### 2. モデル別＆APIキー別の使用量ランキング

チームやサービス、クライアントごとのトークン消費比率を明確に追跡できます。

- **構成比率の横棒グラフ**: 各モデルおよび各 API キーのトークン消費割合と合計呼び出し回数を即座に把握。
- **安全な表示ラベル**: 不可逆ハッシュ化された API キー識別子に、分かりやすい別名（「本番クラスター」「開発環境」など）を設定可能。
- **滑らかなリクエスト曲線**: 分単位・時間単位の API リクエスト推移とエラー率を安定したスプライン曲線で表示。

![モデル別・APIキー別ランキング](assets/screenshots/02-token-rankings.png)

---

### 3. リアルタイムコスト算出と多次元コスト分析

公式の `models.dev` 価格カタログと連携し、正確なドル建て推定費用を算出します。

- **豊富な標準価格表**: 8,000 以上のモデル価格を内蔵し、バックグラウンドでの定期自動更新をサポート。
- **ワンクリックで切り替え**: コスト推移グラフを「モデル別」と「API キー別」の間で瞬時に切り替え可能（再リクエスト不要）。
- **ピーク／オフピーク価格対応**: 時間帯別料金（DeepSeek のピークタイム割引など）のルールに対応し、差額を明確に可視化。
- **オンザフライ計算**: コストはクエリ時に動的に算出されるため、単価を変更しても過去のデータベースを書き換えることなく自動再計算されます。

#### モデル別コスト推移
![モデル別コスト分析](assets/screenshots/03-cost-analysis.png)

#### APIキー別コスト分布
![APIキー別コスト分布](assets/screenshots/04-cost-by-api-key.png)

---

### 4. モデル実測パフォーマンスと柔軟な価格設定

実際の呼び出し速度をモニタリングし、ベンダー契約に応じたカスタム価格を設定できます。

- **2種類の TPS 指標**: 生成 TPS（トークン生成時間ベース）と End-to-End TPS（全体レイテンシベース）を計測し、モデルの真の応答性を把握。
- **初回トークン遅延 (TTFT)**: ミリ秒精度の初回トークンレイテンシを計測。同階層プラグインからの読み取り補完にも対応。
- **単価カスタマイズと時間帯設定**: モデルごとに入力・出力・キャッシュ読み取り単価を個別設定でき、ピーク時間帯の倍率ルールも設定可能。

![モデルパフォーマンス一覧](assets/screenshots/05-model-performance.png)

![モデル別価格設定ダイアログ](assets/screenshots/06-pricing-modal.png)

---

### 5. リクエスト詳細ログとトラブルシューティング

すべての API コールを詳細にトレースし、低速リクエストや異常終了の原因を迅速に特定します。

- **詳細なフィールド追跡**: 実行時刻、モデル名、プロバイダー、API キーラベル、各トークン数、TTFT、全体所要時間を記録。
- **ステータス絞り込み**: 成功／失敗リクエストをワンクリックでフィルタリングし、HTTP 500 や 504 タイムアウトなどのエラーを即座に調査可能。

![リクエスト詳細ログ](assets/screenshots/07-request-logs.png)

---

## クイックスタート

### ステップ 1: プラグインのダウンロードと配置

[Releases ページ](https://github.com/AITNR/cap-token-usage-tracker/releases) からお使いの環境に合わせた共有ライブラリをダウンロードし、CLIProxyAPI の `plugins` ディレクトリに配置します：

| プラットフォームとアーキテクチャ | 配置先パス |
|---|---|
| Linux amd64 | `plugins/linux/amd64/cap-token-usage-tracker.so` |
| Linux arm64 | `plugins/linux/arm64/cap-token-usage-tracker.so` |
| Windows amd64 | `plugins/windows/amd64/cap-token-usage-tracker.dll` |
| macOS (Intel) | `plugins/darwin/amd64/cap-token-usage-tracker.dylib` |
| macOS (Apple Silicon) | `plugins/darwin/arm64/cap-token-usage-tracker.dylib` |

### ステップ 2: 設定の追加

CLIProxyAPI の `config.yaml` にプラグイン設定を追加します：

```yaml
plugins:
  enabled: true
  dir: plugins
  configs:
    cap-token-usage-tracker:
      enabled: true
      retention_days: 365
      api_key_secret: "your-strong-random-secret-key-at-least-32-bytes"
      pricing_auto_refresh: true
      peak_pricing: true
```

### ステップ 3: ダッシュボードへのアクセス

CLIProxyAPI を起動後、ブラウザで以下の URL にアクセスします：

```text
http://127.0.0.1:8317/v0/resource/plugins/cap-token-usage-tracker/dashboard
```

CLIProxyAPI の管理キー（Management Key）を入力するとロックが解除され、すべてのダッシュボード機能を利用できます。

---

## セキュリティとプライバシー方針

- **プロンプトおよび返答内容の完全非保持**: ユーザーの入力テキスト、プロンプト、モデルの生成回答、HTTP ヘッダーなどは一切データベースに保存されません。
- **API キーの平文保存ゼロ**: API キーの平文は保存されず、暗号化ソルト付き HMAC-SHA256 不可逆ハッシュ（指紋）と暗号化世代管理で保護されます。
- **データは完全ローカル完結**: すべての統計データはローカルの `bbolt` 組み込みファイルデータベースにのみ記録され、外部へ送信されることはありません。
- **セッション認証による保護**: データ取得エンドポイントは認証トークンを要求するため、未認証の外部アクセスは `401 Unauthorized` で拒否されます。

---

## 技術アーキテクチャと API 仕様

ビルド手順（CGO / クロスコンパイル）、データベースのゼロ埋めアルゴリズム、RESTful 管理 API の完全な仕様については、技術仕様ドキュメントをご覧ください：

👉 **[技術アーキテクチャ＆API仕様書 (docs/TECHNICAL.md)](docs/TECHNICAL.md)**

---

## ライセンス

本プロジェクトは [MIT License](LICENSE) の下で公開されています。
