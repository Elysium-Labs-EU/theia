# Changelog

All notable changes to theia are documented here.

## [Unreleased]

### Bug Fixes
- Remove the installer staging dir on exit (#106) ([`0210936`](https://github.com/Elysium-Labs-EU/theia/commit/02109361c0b4b75b55824fbf06e09f4bb1f6b50c))


### Features
- Add ADR and GitNexus skills (#117) ([`89713dc`](https://github.com/Elysium-Labs-EU/theia/commit/89713dc6cd118b79e0c555d05f026877ade11097))


### Maintenance
- Bump golang.org/x/mod in the go-dependencies group (#110) ([`007973e`](https://github.com/Elysium-Labs-EU/theia/commit/007973eb701e9c1f59a214e3df0069a2050bb912))
- Bump crate-ci/typos from 1.48.0 to 1.49.0 (#101) ([`6e9236d`](https://github.com/Elysium-Labs-EU/theia/commit/6e9236daa930af9528dcc44b351f74eb0ab45675))
- Drop the dead AGENTS.md rule and anchor the binary ignore (#105) ([`49e14f2`](https://github.com/Elysium-Labs-EU/theia/commit/49e14f254da37c02cc892dac0f96b6c4cd3c3862))
- Pin golangci-lint to v2.12.2 from single source file (#111) ([`248c90b`](https://github.com/Elysium-Labs-EU/theia/commit/248c90bbd352516d7e92701fe86ad383bfe492ee))
- Narrow .claude/ gitignore to local/runtime state only (#116) ([`0498a33`](https://github.com/Elysium-Labs-EU/theia/commit/0498a336737ce7c134a7c7f583ff3fdd8f1b5a0d))


### Miscellaneous
- Route completion install narration through internal/ui (#93) ([`cc0174a`](https://github.com/Elysium-Labs-EU/theia/commit/cc0174af24ea0ed72fe36c846f758a4b917ed88a))

## [0.0.12] - 2026-08-13

### Bug Fixes
- Preflight-check systemctl before installing service (#77) ([`f1f370a`](https://github.com/Elysium-Labs-EU/theia/commit/f1f370a1076708cf336f569d75f3c1c489befa3c))
- Extract Content-Type header literal to constant (#89) ([`f421df4`](https://github.com/Elysium-Labs-EU/theia/commit/f421df4cd51e100932fed132b3faa79e209cf1ed))
- Resolve shellcheck S7679/S7688 findings (#90) ([`63bb1be`](https://github.com/Elysium-Labs-EU/theia/commit/63bb1be73cf8e59a81be6a0cee1d5ed20cc535bd))
- Resolve systemctl from fixed paths, not PATH env (S4036) (#91) ([`02b9281`](https://github.com/Elysium-Labs-EU/theia/commit/02b92812c1a6110c2a2162e4b629169d158d9843))
- Group cobra.Command params and comment blank migrate import (#97) ([`8f36cad`](https://github.com/Elysium-Labs-EU/theia/commit/8f36cadd8f51ceb42606e1a1814fbf83853e8993))
- Locals for positionals, add pkg_manager default (#100) ([`db09366`](https://github.com/Elysium-Labs-EU/theia/commit/db09366e509136731dde5bd7970c315b071b7114))


### CI/CD
- Harden workflow perms, pin action/tool SHAs, enforce HTTPS (#98) ([`18761c8`](https://github.com/Elysium-Labs-EU/theia/commit/18761c805d45a39d4e929bd979faac5ca57ad204))


### Maintenance
- Give each worktree its own golangci-lint cache (#102) ([`7db7e53`](https://github.com/Elysium-Labs-EU/theia/commit/7db7e53c19fd3d78df89ab69de463cdf0f7f22ae))


### Refactoring
- Cut migrations test complexity (#92) ([`9a617d6`](https://github.com/Elysium-Labs-EU/theia/commit/9a617d69dd39f4b0574c53e53c76360b19500899))
- Reduce splitLinesSkippingOverlong complexity (#94) ([`634c17a`](https://github.com/Elysium-Labs-EU/theia/commit/634c17a9935312b0f3b640c9337fc6c1071d438d))
- Extract host filter literal to const (#95) ([`5052c9b`](https://github.com/Elysium-Labs-EU/theia/commit/5052c9b30faff1e2f3095dd17de4650db6398d0b))
- Extract duplicated "(no data)" literal in stats.go (#96) ([`cd78d6e`](https://github.com/Elysium-Labs-EU/theia/commit/cd78d6e6f263ec63b1bc129d17a3942874343301))


### Testing
- Bundle stat-seed params into struct to fix S107 (#99) ([`231c514`](https://github.com/Elysium-Labs-EU/theia/commit/231c51404c19e272d28b49aabd4743669b059a4f))

## [0.0.11-rc.8] - 2026-08-01

### Bug Fixes
- Treat ctx-canceled-during-db-open as graceful shutdown (#74) ([`930f813`](https://github.com/Elysium-Labs-EU/theia/commit/930f813bb5e7df4662a9f857134d54eaff70752a))

## [0.0.11-rc.7] - 2026-08-01

### CI/CD
- Add SonarQube Cloud scan workflow (#70) ([`e85a789`](https://github.com/Elysium-Labs-EU/theia/commit/e85a7898b143a91087cd1a65d7e24f637495ff30))
- Add gitleaks and govulncheck gates (#72) ([`6d62810`](https://github.com/Elysium-Labs-EU/theia/commit/6d6281082ca5993a362d7ef5ae772677af787e0d))
- Add typo checker, file-size/LFS guard, any-over-interface{} lint (#73) ([`0bd5b46`](https://github.com/Elysium-Labs-EU/theia/commit/0bd5b46896971b95fdc8e30001b6dbb3bb731141))


### Features
- Add theia serve-metrics Prometheus exporter (#68) ([`a08c221`](https://github.com/Elysium-Labs-EU/theia/commit/a08c221069cb96108078625d99b9446a341cc2a5))


### Maintenance
- Enable Dependabot for gomod and github-actions (#65) ([`4cc88b3`](https://github.com/Elysium-Labs-EU/theia/commit/4cc88b3a61f57a877e70dff55043eb958dd26eeb))
- Bump github.com/mattn/go-isatty (#71) ([`924ae86`](https://github.com/Elysium-Labs-EU/theia/commit/924ae868acf7b8737f10e3e6d08c4b8e6d94eae1))

## [0.0.11-rc.6] - 2026-07-27

### Miscellaneous
- Pin golangci-lint to CI version via go run (#58) ([`148b62a`](https://github.com/Elysium-Labs-EU/theia/commit/148b62ade3505414e8e5588de53eba76a9b58800))


### Refactoring
- Reuse ui.Confirm instead of local prompt helper (#62) ([`68f53a2`](https://github.com/Elysium-Labs-EU/theia/commit/68f53a28f4dbcf3cc952199f7c2c835661ab7bc5))

## [0.0.11-rc.5] - 2026-07-25

### Bug Fixes
- Fix-issue-36 (#36) ([`276c24d`](https://github.com/Elysium-Labs-EU/theia/commit/276c24d0f421453728c93d9c0a88e874b191b65c))
- Fix-issue-10 (#10) ([`ea6cc07`](https://github.com/Elysium-Labs-EU/theia/commit/ea6cc0712ae31b35e0bb30bc0f211c7fc6acb107))
- Fix-issue-11 (#11) (#30) ([`3c3947b`](https://github.com/Elysium-Labs-EU/theia/commit/3c3947b6f46602b0e15b66d1b76641f6a646ee87))
- Fix-issue-16 (#16) (#33) ([`8fa681c`](https://github.com/Elysium-Labs-EU/theia/commit/8fa681c6a842a4a1624926234b292a902333afd8))
- Fix-issue-17 (#17) (#34) ([`5513533`](https://github.com/Elysium-Labs-EU/theia/commit/55135339c70f4042f82fe76105f31d6d68a21667))
- Fix-issue-18 (#18) (#35) ([`f772c1d`](https://github.com/Elysium-Labs-EU/theia/commit/f772c1d805067e51cc5e9be5f70bd08126de75d3))
- Theia-fix-issue-39 (#39) (#40) ([`36fa25a`](https://github.com/Elysium-Labs-EU/theia/commit/36fa25a29624a7995ffe530f6c97f63799bc60cd))
- Fix-issue-14 (#14) (#31) ([`86208a8`](https://github.com/Elysium-Labs-EU/theia/commit/86208a83d46ebfdfb4db9e9c4a63fdae6cccf02f))
- Fix/pubkey-sync-check-theia (#38) (#50) ([`57fdc65`](https://github.com/Elysium-Labs-EU/theia/commit/57fdc65c4b44b4154afd36b4a2c0f844a277de88))
- Fix/uninstall-confirm-buffer-theia (#28) (#49) ([`53273eb`](https://github.com/Elysium-Labs-EU/theia/commit/53273eb8d75203862a68dae6c9fa11c8b0cc0918))
- Fix/update-refresh-completions-theia (#27) (#48) ([`b42afa3`](https://github.com/Elysium-Labs-EU/theia/commit/b42afa3e17db87214cc997be2a7c93c6219d0c02))
- Fix/daemon-tail-stderr-theia (#15) (#47) ([`135635c`](https://github.com/Elysium-Labs-EU/theia/commit/135635c4cd002c229817de5de29f3c2459161765))
- Feat/stats-api-theia (#1) (#52) ([`44fb4d7`](https://github.com/Elysium-Labs-EU/theia/commit/44fb4d7367532cc9417ecbe24201af7ab7c1794d))
- Fix/lefthook-golangci-lint-theia (#41) (#51) ([`6732c1f`](https://github.com/Elysium-Labs-EU/theia/commit/6732c1f7f9488aaaac936935f39708b782306c3a))
- Start tail at -n 0 to stop restart double-count (#53) ([`0a21dbc`](https://github.com/Elysium-Labs-EU/theia/commit/0a21dbc406b797c01a39482d2462ac36c8cce7e2))
- Run on stock Alpine without bash preinstalled (#55) ([`29fdc86`](https://github.com/Elysium-Labs-EU/theia/commit/29fdc86a60052cc2c117eae0012945d8a5955878))
- Normalize host case so one site is one bucket (#56) ([`b376df4`](https://github.com/Elysium-Labs-EU/theia/commit/b376df48370685d0a60ca584ec1dd82792b8ed57))
- Neutralize terminal escapes in log-derived table fields (#57) ([`6cada5d`](https://github.com/Elysium-Labs-EU/theia/commit/6cada5dc6ab34fda332a96ca5092d427a33ac1fa))
- Survive concurrent daemon startup on same db-path (#54) ([`4fd3593`](https://github.com/Elysium-Labs-EU/theia/commit/4fd35932ace2f6044d116fc3edee7f7470cd641f))


### CI/CD
- Trigger on merge_group for merge queue support (#42) ([`8b3a9d3`](https://github.com/Elysium-Labs-EU/theia/commit/8b3a9d3ca66235b97796d3a70c6fd54decabf05c))


### Documentation
- Add CONTRIBUTING.md (#43) ([`19b4be2`](https://github.com/Elysium-Labs-EU/theia/commit/19b4be23b7288a1bf074645ba737ebcf53bf88cf))

## [0.0.11-rc.4] - 2026-07-20

### Bug Fixes
- Derive prerelease flag from tag suffix instead of hardcoding false (#9) ([`beda050`](https://github.com/Elysium-Labs-EU/theia/commit/beda050ebc657f57f368bf48ceb2aa49fb0a26d8))


### CI/CD
- Add coverage floor gate, close release-time crap/coverage gap ([`9c22da9`](https://github.com/Elysium-Labs-EU/theia/commit/9c22da9c8577722d25a1e7e0731545e7431ebc9d))


### Features
- Sign releases and verify signatures (F-001/F-002) ([`2af61f4`](https://github.com/Elysium-Labs-EU/theia/commit/2af61f4cd0a18037cb6250c99197f0134051d5d6))


### Maintenance
- Rotate release-signing public key ([`2e0ef83`](https://github.com/Elysium-Labs-EU/theia/commit/2e0ef838f335b8a25b3e89fdfaac1ab345b58418))

## [0.0.10] - 2026-07-19

### Bug Fixes
- Correct module path to codeberg.org/Elysium_Labs/theia ([`046bcf7`](https://github.com/Elysium-Labs-EU/theia/commit/046bcf73d9cbd58a234e5ef7246f42314090002d))
- Dedupe unique visitors across query range instead of summing per-bucket counts ([`0498d20`](https://github.com/Elysium-Labs-EU/theia/commit/0498d209bcd7c4754cddc0c1332f6d97f363a3f9))
- Skip merge commits and changelog-bump commits from changelog ([`27b7a0f`](https://github.com/Elysium-Labs-EU/theia/commit/27b7a0f9bc6697378417da11ebdf754a52a073f9))
- Use full GitHub URL for osv-scanner-action, not mirrored on Forgejo ([`26f94cf`](https://github.com/Elysium-Labs-EU/theia/commit/26f94cf0eccc5ca862c650b7cd7f8847e512aa9b))
- Allow GOTOOLCHAIN auto-upgrade for osv-scanner install ([`cca5379`](https://github.com/Elysium-Labs-EU/theia/commit/cca5379dc2a285f5559e3f33fe9ae728aa6f1e2d))


### CI/CD
- Add OSV scanner workflow for PRs to main ([`f5719e8`](https://github.com/Elysium-Labs-EU/theia/commit/f5719e8a78df8f3483855a78dbb91ede46c59f21))
- Run osv-scanner CLI directly instead of GitHub Action wrapper ([`d998708`](https://github.com/Elysium-Labs-EU/theia/commit/d998708cd48057ac2663f62022b8c405b1b82580))
- Run Go jobs in container with named-volume caches ([`d18864a`](https://github.com/Elysium-Labs-EU/theia/commit/d18864ae1abea5af06b21f9333bb04e655007ed8))
- Drop dead permissions field from Forgejo workflows ([`747d499`](https://github.com/Elysium-Labs-EU/theia/commit/747d4990ff641c83d237de49333897fa90b59208))
- Tune OSV scan (deps-only PRs + weekly cron, prebuilt scanner) ([`3dfb24f`](https://github.com/Elysium-Labs-EU/theia/commit/3dfb24f6f382c738ddc15e04ea82a71344d3848e))
- Bump actions/checkout to v7 (current latest) ([`0fa4211`](https://github.com/Elysium-Labs-EU/theia/commit/0fa421155d4cb8d243a54bddb1a2af6c333dfbb4))


### Documentation
- Point README install/clone commands at Codeberg instead of GitHub mirror ([`4f37a73`](https://github.com/Elysium-Labs-EU/theia/commit/4f37a73816822779061d77bf81e45d251aeb131f))
- Add Codeberg badge and canonical-repo note to README ([`95da15d`](https://github.com/Elysium-Labs-EU/theia/commit/95da15d330546acf0276ddfa366b6f3639a2c24d))
- Add theia logo to README ([`fedc4fa`](https://github.com/Elysium-Labs-EU/theia/commit/fedc4fa85458103e5b08617a0819153c4007c93f))


### Features
- Add top-level --version/-v flag ([`6ee776b`](https://github.com/Elysium-Labs-EU/theia/commit/6ee776bae164e2aa75897fca68a004fd9d5e3736))
- Point installer and self-updater at GitHub releases ([`0a0a9af`](https://github.com/Elysium-Labs-EU/theia/commit/0a0a9af679bf0b9a4dd27471d04543c72ed195c7))


### Maintenance
- Remove dead GitHub-Actions-only permissions block from workflows ([`24fd951`](https://github.com/Elysium-Labs-EU/theia/commit/24fd951b79ee71edf0179ce47ce30d83f5c8d7bc))
- Add go-crap as a hard-blocking CI gate ([`a55f70d`](https://github.com/Elysium-Labs-EU/theia/commit/a55f70d7d96e2c1e1c1a35ffcb00b2008d8f261f))
- Gate pre-commit on go-crap CRAP score too ([`3fe9cde`](https://github.com/Elysium-Labs-EU/theia/commit/3fe9cde12a1b7a09de13133173320f33340bbfa5))
- Make go-crap gate change-scoped instead of whole-repo ([`94cbcfd`](https://github.com/Elysium-Labs-EU/theia/commit/94cbcfdb68e54d7a11a6e4400be6a2281b23887f))
- Run go-crap gate on pre-push, not pre-commit ([`9525c0e`](https://github.com/Elysium-Labs-EU/theia/commit/9525c0ef02a01207fa80abed2b1adbfb80a2bea4))
- Add GitHub issue-form templates for parity with Forgejo ([`03a178b`](https://github.com/Elysium-Labs-EU/theia/commit/03a178b4acc8f7eeb1a5832490503f0ac3c6212e))
- Migrate repo identity from Codeberg to GitHub ([`6e85fbd`](https://github.com/Elysium-Labs-EU/theia/commit/6e85fbd20991da82887719377811a62b4e1cc4e0))


### Miscellaneous
- Fix import grouping after module path rename ([`d25b524`](https://github.com/Elysium-Labs-EU/theia/commit/d25b524e82e9d247ccbf6b3a9afe90fd1e74d7bb))
- Require go 1.26.5 so go-crap (needs >=1.26.2) installs in CI ([`6075f2d`](https://github.com/Elysium-Labs-EU/theia/commit/6075f2d77be581f1dcc782834292268a121ef1ce))
- Stage release download in a private mktemp dir ([`c6c99a2`](https://github.com/Elysium-Labs-EU/theia/commit/c6c99a275784ad0f541dc04a80f2ff2051a17cf7))
- Bring in GitHub main's history to make the migration PR mergeable ([`bd72e5e`](https://github.com/Elysium-Labs-EU/theia/commit/bd72e5e5819ecf76c3337e6dbab1710ceb9f0c35))


### Refactoring
- Extract helpers to cut runUpdate/downloadFile CRAP score ([`ac2b48a`](https://github.com/Elysium-Labs-EU/theia/commit/ac2b48a8387bc896d8fe8f06077d403620c84ea7))


### Testing
- Extract shared scenario helpers to cut CRAP score ([`8b6ce18`](https://github.com/Elysium-Labs-EU/theia/commit/8b6ce18e3ab6dd9f46f2f0010d784aed6cc95d25))

## [0.0.11-rc.3] - 2026-07-14

### Bug Fixes
- Correct next-steps numbering when service was already running ([`e2435ab`](https://github.com/Elysium-Labs-EU/theia/commit/e2435aba8534847f238314b176da4e7ca2b5a8bc))
- Open sqlite with WAL + busy_timeout, matching eos ([`a1526b4`](https://github.com/Elysium-Labs-EU/theia/commit/a1526b431b56d01460a4c1bc3e2dc3c7c291f3c2))

## [0.0.11-rc.2] - 2026-07-14

### Bug Fixes
- Pass the daemon subcommand in theia.service's ExecStart ([`71661a6`](https://github.com/Elysium-Labs-EU/theia/commit/71661a6a779ea6d06fa2b18d26cfc6945e89ff65))

## [0.0.11-rc.1] - 2026-07-14

### Bug Fixes
- Fix stats linting: pointers, error handling, flush propagation

Pass statsReport by pointer to avoid copies, discard fmt write
errors explicitly, propagate tabwriter flush error instead of
swallowing it in defer.

Add CLI command tests for main, root, stats, and version. ([`c51e527`](https://github.com/Elysium-Labs-EU/theia/commit/c51e52709c51859c25b3753e4ac0c7f3e558ee08))
- Detect host arch for git-cliff install ([`c1743a8`](https://github.com/Elysium-Labs-EU/theia/commit/c1743a82b64f2c55f940680f20a773a00bf278df))


### CI/CD
- Add Forgejo workflow templates, mirroring eos ([`234f536`](https://github.com/Elysium-Labs-EU/theia/commit/234f53619432db7e564de16bfaede27dddb3600d))
- Add issue/PR templates, mirroring eos ([`f5e5d4f`](https://github.com/Elysium-Labs-EU/theia/commit/f5e5d4ff764612f16585bf4f42888c8190b640b4))


### Features
- Adds Apache 2.0 license and updates readme ([`0d95e59`](https://github.com/Elysium-Labs-EU/theia/commit/0d95e5904314a5975188d6d8b7f14267a0be9cbb))
- Add make list command and set help as default target ([`12f1cb1`](https://github.com/Elysium-Labs-EU/theia/commit/12f1cb1db82d0176e01d8e5f4b2ed1a48e0f4be1))
- Fall back to full help on bare invocation, show version ([`2d6aebf`](https://github.com/Elysium-Labs-EU/theia/commit/2d6aebf331f78f444c5f17ae52b8cc14a3a8679d))
- Rewrite install.sh with styled output and --local/--yes flags ([`8d9f1ff`](https://github.com/Elysium-Labs-EU/theia/commit/8d9f1ff7d32db2e25a234e67df1d10e87bb716d8))
- Add interactive shell completion, ported from eos ([`1e4c6b6`](https://github.com/Elysium-Labs-EU/theia/commit/1e4c6b6d1f2777b56473afe145b7dde2d141d4dc))
- Add system update/uninstall commands, fix install.sh binary swap ([`1427d40`](https://github.com/Elysium-Labs-EU/theia/commit/1427d4055cbd0442bff28678a4f1e573ee0995f8))


### Improvements
- Updates README and release text ([`db770bc`](https://github.com/Elysium-Labs-EU/theia/commit/db770bc5fbfe201b37bd44ad59ed37d1d842f602))


### Miscellaneous
- Converts theia to a proper CLI with cobra commands

Adds daemon, stats, and version subcommands. The stats command reads
analytics from SQLite so users no longer need raw sqlite3 access.
Includes query package with tests and updated README for CLI usage. ([`e9aae59`](https://github.com/Elysium-Labs-EU/theia/commit/e9aae59de7606e834361c784cc5b1c02372ed75e))

## [0.0.9] - 2026-01-15

### Improvements
- Updates README 'How It Works' section ([`12b2ec2`](https://github.com/Elysium-Labs-EU/theia/commit/12b2ec27849ac0b1e09c76e05f2c5ffbaf70db9b))
- Updates README various sections ([`841d66d`](https://github.com/Elysium-Labs-EU/theia/commit/841d66dd8b4ed787c652753f7130f4c8e6d9b9b6))


### Miscellaneous
- Embeds the migrations files into the binary ([`8385b16`](https://github.com/Elysium-Labs-EU/theia/commit/8385b165ddea857d4c75c51fb8256627e2be61be))

## [0.0.8] - 2026-01-06

### Features
- Adds IsStatic detection data into database ([`f7b1205`](https://github.com/Elysium-Labs-EU/theia/commit/f7b12054f16a4d573ea011a76956faf07feb6fe2))
- Adds database migrations and migration tests ([`113d78c`](https://github.com/Elysium-Labs-EU/theia/commit/113d78c8a1756df71eb2b27660b36e97fbb37c61))


### Improvements
- Improves README examples and installation commands ([`43331a4`](https://github.com/Elysium-Labs-EU/theia/commit/43331a40d5077e069624037473ec8475b3a8d463))

## [0.0.7] - 2026-01-05

### Features
- Adds option to tracking multiple hosts ([`1b239c6`](https://github.com/Elysium-Labs-EU/theia/commit/1b239c64bd35d1f9a353869aacf2de98f93c98d3))

## [0.0.6] - 2026-01-03

### Features
- Adds integration test for the happy path ([`3d4847c`](https://github.com/Elysium-Labs-EU/theia/commit/3d4847c87990595ff772b01b81c0a52b460ca817))


### Miscellaneous
- Makes this GDPR complaint + expands integration testing suite ([`726fe67`](https://github.com/Elysium-Labs-EU/theia/commit/726fe67422c46429f3a93c16d1310539e54de098))

## [0.0.5] - 2025-12-22

### Features
- Add wget installation command to README ([`3116693`](https://github.com/Elysium-Labs-EU/theia/commit/31166930cf46ec402c57e2a64ad6c42bca072523))
- Adds automatic database clean up of entries older than 60 days ([`b717f3b`](https://github.com/Elysium-Labs-EU/theia/commit/b717f3b3c2b59fd69b6b442a74cd5187fe593a21))

## [0.0.4] - 2025-12-22

### Improvements
- Improves page view recording by taking note of status code and bytes sent to allow for filtering ([`a6b6db4`](https://github.com/Elysium-Labs-EU/theia/commit/a6b6db492c965e5c0c08d3ff7677c9403ee99cff))

## [0.0.3] - 2025-12-22

### Features
- Adds sqlite3 dependency installation in install script ([`288d90c`](https://github.com/Elysium-Labs-EU/theia/commit/288d90cf0c69e0bd16dd335d9b31c5e04c5187c8))


### Improvements
- Updates README with improved sql commands ([`669aa94`](https://github.com/Elysium-Labs-EU/theia/commit/669aa9455924fe54e48aeb4d24a1ad75435f7dcf))
- Updates theia naming convention in Readme ([`26a4d63`](https://github.com/Elysium-Labs-EU/theia/commit/26a4d630b8556856e124689a2c026e8f1e9a95b3))
- Improves output for pageview tracking by detecting bots and static assets ([`3eee044`](https://github.com/Elysium-Labs-EU/theia/commit/3eee044a2c82e983d1ab4c639f1e47f9d1e141dd))

## [0.0.2] - 2025-12-22

### Bug Fixes
- Fixes invalid sqlite package reference ([`6484885`](https://github.com/Elysium-Labs-EU/theia/commit/6484885e52d97b174f3eeb6f60122b8e992a5829))


### Miscellaneous
- Upgrades install.sh for download tool detection ([`4cf5405`](https://github.com/Elysium-Labs-EU/theia/commit/4cf540548060903687c0c1b1d2b4f4079b2fc0cd))
- Changes sqlite dependency to Go pure variant + updates readme with improved installation commands ([`226b0d7`](https://github.com/Elysium-Labs-EU/theia/commit/226b0d7e0427cf4b85d6f5beac4b7c3c3b1aa2b5))

## [0.0.1] - 2025-12-22

### Features
- Adds install script and build and release step ([`624b932`](https://github.com/Elysium-Labs-EU/theia/commit/624b93260849ff2c2ee4b72e4a2eb13734a09b3f))


### Miscellaneous
- Initial version of server side pageview tracking ([`d956aab`](https://github.com/Elysium-Labs-EU/theia/commit/d956aabd92f8dc70dcd7cff606242c77afdb419e))

