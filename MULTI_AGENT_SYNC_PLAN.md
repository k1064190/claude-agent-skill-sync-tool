# AI Agent Sync Tool Unification Plan

## 1. 개요 (Overview)
현재 `claude-agent-skill-sync-tool`(Go 기반 TUI)과 `server_settings/vibe_syncing/sync.sh`(Bash 스크립트 기반)로 이원화되어 있는 설정/스킬/에이전트 동기화 프로세스를 하나의 Go 기반 단일 도구로 통합합니다.
이를 통해 Claude Code 뿐만 아니라 **Codex, Gemini CLI, Opencode** 등 다중 AI 에이전트의 스킬과 전역 설정(Prompt, Rules 등)을 한 번에 제어할 수 있도록 만듭니다.

---

## 2. As-Is vs To-Be

### As-Is (현재 상태)
- `claude-agent-skill-sync-tool`: `skills`, `agents`, `rules`를 TUI로 선택하여 `~/.claude/` (또는 `./.claude/`)에 Symlink 생성. (Claude 전용)
- `sync.sh`, `sync_skills.sh`: `common/` + `claude/`, `codex/` 등의 디렉토리 내용을 `rsync`로 복사하고, `common/AGENTS.md`와 각 플랫폼별 `.md`를 `cat`으로 합쳐서(Concatenation) 최종 `CLAUDE.md` 등을 생성.

### To-Be (목표 상태)
- `ai-agent-sync-tool` (Go 도구 개선):
  1. **플랫폼 선택 단계 추가:** TUI 시작 시 동기화할 플랫폼(Claude, Gemini, Codex, Opencode)을 다중 선택.
  2. **통합 Symlink 라우팅:** 선택한 스킬/에이전트를 각 플랫폼의 타겟 디렉토리(`~/.claude/`, `~/.gemini/`, `~/.codex/`)에 동시 연결.
  3. **Config Builder 내장:** `sync.sh`가 수행하던 `cat A + B > C` 로직을 Go 도구 내에 내장. `common` 규칙과 각 플랫폼 특화 규칙을 합쳐서 설정 파일을 자동 생성.

---

## 3. 디렉토리 구조 개편안 (Directory Restructuring)

`server_settings/vibe_syncing` (또는 통합 Source Root)의 구조를 AI 에이전트 범용으로 재편합니다.

```text
source-root/
├── skills/           # 모든 AI가 공통으로 참조할 스킬 디렉토리
├── agents/           # 에이전트 정의 파일들
├── rules/            # 프로젝트/유저별 규칙
└── templates/        # 💡 신규: 기존 sync.sh의 build_agents 역할을 대체
    ├── common.md     # 공통 AGENTS.md / Core Rules
    ├── claude.md     # Claude 전용 규칙
    ├── gemini.md     # Gemini 전용 규칙
    └── codex.md      # Codex 전용 규칙
```

---

## 4. Go 프로그램(`claude-agent-skill-sync-tool`) 수정 계획 (Implementation Steps)

### Step 1: 플랫폼 매핑 및 선택 UI 추가 (`go/internal/config`, `go/internal/tree`)
- Bubbletea 기반 UI에 **"어떤 AI 에이전트 환경에 동기화할 것인가?"** (다중 선택 체크박스) 화면 추가.
- 각 플랫폼별 타겟 디렉토리 맵핑 로직 추가:
  - `Claude` -> `~/.claude/` (또는 프로젝트 `./.claude/`)
  - `Gemini` -> `~/.gemini/` (또는 프로젝트 `./.gemini/`)
  - `Codex` -> `~/.codex/` (또는 프로젝트 `./.codex/`)

### Step 2: 템플릿 빌더(Template Builder) 로직 구현 (`go/internal/sync/builder.go`)
- 기존에 `rsync`와 `cat`으로 하던 파일 병합 방식을 Go 로직으로 대체합니다.
- 선택된 플랫폼에 대해 다음 로직 수행:
  - Claude 선택 시: `templates/common.md` + `templates/claude.md` -> `~/.claude/CLAUDE.md` (파일 생성)
  - Gemini 선택 시: `templates/common.md` + `templates/gemini.md` -> `~/.gemini/GEMINI.md` (파일 생성)

### Step 3: Symlink 동기화 엔진 개선 (`go/internal/sync`)
- TUI 트리에서 선택된 `skills`, `agents`, `rules`를 체크된 **모든** 플랫폼 타겟 디렉토리에 병렬 또는 순차적으로 Symlink를 생성하거나 삭제하도록 로직 반복문(loop) 수정.

### Step 4: 마이그레이션 및 레거시 제거
- `server_settings/vibe_syncing/sync*.sh` 스크립트 제거 (폐기).
- 통합된 Go 도구를 빌드하고 `~/.local/bin/ai-sync` 등으로 이름 변경하여 배포.

---

## 5. 실행을 위한 제안 (How to proceed)

이 계획에 동의하시면, 다음 단계 중 하나를 선택하여 진행을 지시해 주십시오:
1. **디렉토리 마이그레이션**: `server_settings/vibe_syncing` 내의 파일들을 위 구조(`skills`, `templates` 등)로 재배치.
2. **Go 코드 수정 시작**: `claude-agent-skill-sync-tool/go` 폴더 내의 소스 코드를 수정하여 **플랫폼 선택 UI**와 **템플릿 빌드 기능**을 개발.
3. **템플릿 병합 로직 작성**: `common/AGENTS.md`와 각 플랫폼별 `.md`를 쉽게 합치기 위한 통합 템플릿 폴더 구성.