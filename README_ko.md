# CAP Token Usage Tracker

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![CLIProxyAPI](https://img.shields.io/badge/CLIProxyAPI-Plugin-success.svg)](https://github.com/router-for-me/CLIProxyAPI)

**[English](README.md)** | **[简体中文](README_zh.md)** | **[日本語](README_ja.md)** | **[Français](README_fr.md)** | **[한국어](README_ko.md)**

---

**CAP Token Usage Tracker**는 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)를 위한 경량 영구 토큰 사용량 및 비용 분석 플러그인입니다. 공식 플러그인 인터페이스와 직접 연동되어 요청 메타데이터와 분 단위 집계 데이터를 로컬 데이터베이스에 안전하게 기록하고, 직관적이고 반응이 빠른 대시보드를 제공합니다.

복잡한 모니터링 인프라 없이도 토큰 소비 구성, 프롬프트 캐시(Prompt Cache) 적중률, 실시간 비용 추정, 모델별 실측 지연 시간을 한눈에 파악할 수 있습니다.

---

## 핵심 기능 및 대시보드 화면

### 1. 토큰 사용량 분석 및 프롬프트 캐시 효율 모니터링

전체 트래픽 규모와 토큰 구성을 실시간으로 확인하고, 컨텍스트 캐싱을 통한 처리 속도 및 비용 절감 효과를 정확하게 측정합니다.

- **토큰 구성 상세 시각화**: 입력 토큰, 출력 토큰, 캐시 적중 토큰을 명확히 구분하여 표시합니다.
- **캐시 적중률(Cache Hit Rate) 측정**: 전체 프롬프트 캐시 적중률을 계산하여 컨텍스트 재사용 효과를 평가합니다.
- **유연한 시간 단위 집계**: 분, 시간, 일 단위 전환을 지원하며, 데이터가 희소한 구간도 0으로 채워져 타임라인 왜곡 없이 정확한 추세를 보여줍니다.

![토큰 사용량 및 캐시 분석](assets/screenshots/01-token-overview.png)

---

### 2. 모델 및 API 키별 소비 순위

팀, 서비스, 개발자별 토큰 소비 비율을 손쉽게 추적하고 관리할 수 있습니다.

- **수평 비율 랭킹 바**: 활성 모델 및 API 키별 사용 비율과 총 호출 횟수를 실시간으로 비교합니다.
- **가독성 높은 별칭 라벨**: 역산이 불가능한 API 키 지문에 사람이 읽기 쉬운 이름(예: `Production-Agent-Cluster`, `Dev-Workspace-IDE`)을 지정할 수 있습니다.
- **부드러운 요청 트래픽 곡선**: 분/시간 단위의 API 요청량과 실패율 추이를 스플라인 곡선으로 안정감 있게 표시합니다.

![모델 및 API 키 소비 순위](assets/screenshots/02-token-rankings.png)

---

### 3. 실시간 비용 계산 및 다차원 비용 귀속

공식 `models.dev` 가격 카탈로그와 연동하여 예상 비용을 달러(USD) 기준으로 산출합니다.

- **내장 정가 카탈로그**: 8,000개 이상의 모델 기준 요율을 내장하고 있으며, 백그라운드 자동 갱신을 지원합니다.
- **원클릭 차원 전환**: 추가 서버 요청 없이 누적 비용 히스토그램을 **모델별**과 **API 키별**로 즉시 전환할 수 있습니다.
- **피크 / 오프피크 요금제 반영**: 시간대별 차등 요금제(DeepSeek 피크 플랜 등)를 지원하여 시간대별 비용 변동을 정확히 반영합니다.
- **동적 계산 방식**: 비용은 조회 시점에 즉시 계산되므로, 단가를 수정하더라도 과거 데이터베이스를 재작성할 필요 없이 즉시 업데이트됩니다.

#### 모델별 비용 추이
![모델별 비용 분석](assets/screenshots/03-cost-analysis.png)

#### API 키별 비용 분포
![API 키별 비용 분포](assets/screenshots/04-cost-by-api-key.png)

---

### 4. 모델 실측 성능 지표 및 유연한 단가 설정

실제 모델 호출 속도를 측정하고 계약 요율에 맞게 개별 단가를 맞춤 설정할 수 있습니다.

- **이중 TPS 지표**: 순수 생성 속도를 나타내는 **생성 TPS**와 전체 지연 시간을 반영하는 **End-to-End TPS**를 함께 제공합니다.
- **첫 토큰 지연 시간 (TTFT)**: 밀리초 단위의 첫 글자 응답 지연을 측정하며, 동일 호스트 내 플러그인과의 연동 보완도 지원합니다.
- **단가 및 피크 시간대 설정**: 모델별로 입력, 출력, 캐시 읽기/쓰기 단가를 직접 입력하거나 피크 시간대 요율 배수를 구성할 수 있습니다.

![모델 성능 지표](assets/screenshots/05-model-performance.png)

![단가 설정 팝업](assets/screenshots/06-pricing-modal.png)

---

### 5. 상세 요청 로그 및 트러블슈팅

모든 API 호출의 상세 기록을 확인하여 병목 지점과 장애를 신속하게 추적할 수 있습니다.

- **세부 필드 추적**: 타임스탬프, 모델명, 제공자, API 키 라벨, 토큰 상세 수치, TTFT, 총 소요 시간을 기록합니다.
- **상태별 필터링**: 성공 또는 실패한 요청을 즉시 필터링하여 HTTP 500 오류나 504 타임아웃 등의 장애 원인을 빠르게 진단합니다.

![상세 요청 로그](assets/screenshots/07-request-logs.png)

---

## 빠른 시작

### 1단계: 플러그인 다운로드 및 설치

[Releases 페이지](https://github.com/AITNR/cap-token-usage-tracker/releases)에서 운영체제 및 아키텍처에 맞는 라이브러리 파일을 다운로드하여 CLIProxyAPI의 `plugins` 디렉터리에 넣습니다:

| 운영체제 및 아키텍처 | 설치 경로 |
|---|---|
| Linux amd64 | `plugins/linux/amd64/cap-token-usage-tracker.so` |
| Linux arm64 | `plugins/linux/arm64/cap-token-usage-tracker.so` |
| Windows amd64 | `plugins/windows/amd64/cap-token-usage-tracker.dll` |
| macOS (Intel) | `plugins/darwin/amd64/cap-token-usage-tracker.dylib` |
| macOS (Apple Silicon) | `plugins/darwin/arm64/cap-token-usage-tracker.dylib` |

### 2단계: 설정 파일 추가

CLIProxyAPI의 `config.yaml`에 플러그인 설정을 추가합니다:

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

### 3단계: 대시보드 접속

CLIProxyAPI를 실행한 뒤, 브라우저에서 아래 대시보드 URL에 접속합니다:

```text
http://127.0.0.1:8317/v0/resource/plugins/cap-token-usage-tracker/dashboard
```

CLIProxyAPI의 관리 키(Management Key)를 입력하면 잠금이 해제되고 모든 지표와 차트를 이용할 수 있습니다.

---

## 보안 및 개인정보 보호

- **프롬프트 및 응답 본문 미수집**: 사용자의 입력 프롬프트, 요청 본문, 모델의 생성 텍스트, 민감한 요청 헤더는 절대로 데이터베이스에 기록되지 않습니다.
- **API 키 평문 저장 방지**: API 키 평문은 저장되지 않으며, 솔트가 적용된 HMAC-SHA256 비가역 지문과 암호화 세대 관리로 보호됩니다.
- **100% 로컬 저장소 유지**: 모든 수집 데이터는 로컬 임베디드 `bbolt` 데이터베이스 파일에만 보관되며 외부 서버로 전송되지 않습니다.
- **세션 기반 인증 격리**: 데이터 엔드포인트는 인증된 세션 토큰을 필수로 요구하며, 무단 외부 접근 시 `401 Unauthorized`로 차단됩니다.

---

## 기술 아키텍처 및 API 명세

CGO 빌드 명령어, 크로스 컴파일 절차, 제로 채우기(Zero-fill) 알고리즘 및 전체 RESTful 관리 API 명세는 전용 기술 문서를 참고하시기 바랍니다:

👉 **[기술 아키텍처 및 API 상세 문서 (docs/TECHNICAL.md)](docs/TECHNICAL.md)**

---

## 라이선스

이 프로젝트는 [MIT License](LICENSE)에 따라 배포됩니다.
