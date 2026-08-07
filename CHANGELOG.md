# Changelog

## [1.1.0](https://github.com/abinnovision/ssh-pve/compare/v1.0.0...v1.1.0) (2026-08-07)


### Features

* add fuzzy type-ahead search to VM list ([#2](https://github.com/abinnovision/ssh-pve/issues/2)) ([e82bc35](https://github.com/abinnovision/ssh-pve/commit/e82bc3568680e2971fedc937d6a495209b1f10f0))

## 1.0.0 (2026-08-04)


### Features

* add .gitignore and Makefile ([540138f](https://github.com/abinnovision/ssh-pve/commit/540138fab2ccf15cd42ce344c72cc002b0820b63))
* add cache package for VM inventory persistence ([ee4b510](https://github.com/abinnovision/ssh-pve/commit/ee4b51034ba1414cd03da3d559469e4e54b2b6c3))
* add config package for TUI on-disk YAML store ([74b0114](https://github.com/abinnovision/ssh-pve/commit/74b0114781e5c8c89c9d6d00df81fd8095385361))
* add main entry point wiring tui.Run ([b1332a8](https://github.com/abinnovision/ssh-pve/commit/b1332a83608b7c4ec568656023103d08f6888d22))
* add pve package for cluster-wide VM inventory with guest-agent IPs ([a80731a](https://github.com/abinnovision/ssh-pve/commit/a80731ae3d65ff59b6a9b1d19884d6c33ef5200d))
* add tui package with onboarding and VM list screens ([ac3ad4f](https://github.com/abinnovision/ssh-pve/commit/ac3ad4f14f4dca429047dee4980761b5a9da82c1))
* add Verify TLS checkbox to onboarding form ([da392ee](https://github.com/abinnovision/ssh-pve/commit/da392ee28dda2fdcc68f972eff96240c8325f040))
* pin keybindings below a scrollable onboarding pane ([1d06f76](https://github.com/abinnovision/ssh-pve/commit/1d06f761380050efd97bf4470432122f7fe685f3))
* scrollable onboarding form and filled input field backgrounds ([af5228a](https://github.com/abinnovision/ssh-pve/commit/af5228af1adb8e8133620ab0d2964b5483609dc0))
* show cached VMs instantly while fetching fresh data in background ([2830b69](https://github.com/abinnovision/ssh-pve/commit/2830b69fc6b10e7f10d7cec87c66d158b4101418))


### Bug Fixes

* correct PVE permission guidance in onboarding and auth error ([768b316](https://github.com/abinnovision/ssh-pve/commit/768b3168b78a98101a37f6af6b60c5d8b890c8f3))
* normalize PVE API URL path and surface actionable auth error ([cd6e129](https://github.com/abinnovision/ssh-pve/commit/cd6e129d67ee4d800862e39c1070f5b4b451750e))
* truncate long IP lists in VM list with ellipsis ([d88562e](https://github.com/abinnovision/ssh-pve/commit/d88562e24c8c2f874d00a5d73b2b41387c929482))
