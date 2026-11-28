# Changelog

## [0.8.3-rc.1](https://github.com/agentic-software-engineers/o3s/compare/v0.8.2...v0.8.3-rc.1) (2026-11-28)


### Features

* **cage:** bridge the cage's loopback to the host's sockets ([#353](https://github.com/agentic-software-engineers/o3s/issues/353)) ([12030cd](https://github.com/agentic-software-engineers/o3s/commit/12030cdde8d496e42a84b0760346abdaa11a0b91))
* **cage:** feed the relay's services from the home's files ([#349](https://github.com/agentic-software-engineers/o3s/issues/349)) ([ca9395c](https://github.com/agentic-software-engineers/o3s/commit/ca9395c1fde31ca9ab6311ec918ba8644de77615))
* **cage:** keep one host process per home running ([#351](https://github.com/agentic-software-engineers/o3s/issues/351)) ([c0337ba](https://github.com/agentic-software-engineers/o3s/commit/c0337ba51ffe94ed8c99351755898ed716c4637c))
* **cage:** run the cage without a network on the host engine ([#366](https://github.com/agentic-software-engineers/o3s/issues/366)) ([651662c](https://github.com/agentic-software-engineers/o3s/commit/651662c11fce983fe23025934ee030f47b013a0a))
* **cli:** check that the host runs a VM inside the home's VM ([#325](https://github.com/agentic-software-engineers/o3s/issues/325)) ([cebd078](https://github.com/agentic-software-engineers/o3s/commit/cebd0788112ef9c75a2e7e473c41bcd1828333a3))
* **dns:** answer the cage's names through the allowlist ([#346](https://github.com/agentic-software-engineers/o3s/issues/346)) ([d3aecde](https://github.com/agentic-software-engineers/o3s/commit/d3aecdedeb9a01bea4696bef3e974f359089c7a0))
* **docs:** add the overview page for the central docs ([#369](https://github.com/agentic-software-engineers/o3s/issues/369)) ([77f5dd2](https://github.com/agentic-software-engineers/o3s/commit/77f5dd2575ed9b554dc7d49e4de4c644beab4547))
* **docs:** name o3s in full in the product switcher ([#370](https://github.com/agentic-software-engineers/o3s/issues/370)) ([eab3d7a](https://github.com/agentic-software-engineers/o3s/commit/eab3d7aa7e850f361de04732d11883d1a8aca7be))
* **proxy:** refuse private and metadata addresses from the first packet ([#338](https://github.com/agentic-software-engineers/o3s/issues/338)) ([ce2627b](https://github.com/agentic-software-engineers/o3s/commit/ce2627b25cef2d0e13aa8794d6593bcb98884817))
* **proxy:** serve the allow-list proxy over the relay ([#334](https://github.com/agentic-software-engineers/o3s/issues/334)) ([2f2fc48](https://github.com/agentic-software-engineers/o3s/commit/2f2fc4827173390b5aa9837d987f84333d5cba9c))
* **proxy:** swap placeholder secrets for their bound host and slot ([#336](https://github.com/agentic-software-engineers/o3s/issues/336)) ([d6670ea](https://github.com/agentic-software-engineers/o3s/commit/d6670eaad07e3e7d5f1e3e27c553f73101cc7fa2))
* **relay:** carry the cage's connections to o3s over one transport ([#332](https://github.com/agentic-software-engineers/o3s/issues/332)) ([4edad6e](https://github.com/agentic-software-engineers/o3s/commit/4edad6e765b23e2ea8a84e58cae3fa1694a47f36))


### Bug Fixes

* **cage:** deliver the shutdown answer before the control socket closes ([#379](https://github.com/agentic-software-engineers/o3s/issues/379)) ([479e860](https://github.com/agentic-software-engineers/o3s/commit/479e8603d39038b0e0e6415fed2e2f681459aa76))
* **cage:** start a new host process once the old one lets go of the home ([#391](https://github.com/agentic-software-engineers/o3s/issues/391)) ([58e1c6c](https://github.com/agentic-software-engineers/o3s/commit/58e1c6c77660ed212c18fdfabb4177b939359226))
* **deps:** update ghcr.io/mitmproxy/mitmproxy docker tag to v12.3.1 ([#359](https://github.com/agentic-software-engineers/o3s/issues/359)) ([d994d6c](https://github.com/agentic-software-engineers/o3s/commit/d994d6c3649a9f77d636d0c80f7b90de316af07a))
* **deps:** update go modules (minor) ([#360](https://github.com/agentic-software-engineers/o3s/issues/360)) ([14a0113](https://github.com/agentic-software-engineers/o3s/commit/14a0113e3df3132d6bde1ea7520b9249a2984aa4))
* **deps:** update go modules (patch) ([#355](https://github.com/agentic-software-engineers/o3s/issues/355)) ([60a080e](https://github.com/agentic-software-engineers/o3s/commit/60a080eba9a1ad33c34a67721a4f7d3e51c3e553))
* **deps:** update module charm.land/bubbles/v2 to v2.2.2 ([#380](https://github.com/agentic-software-engineers/o3s/issues/380)) ([b41ede9](https://github.com/agentic-software-engineers/o3s/commit/b41ede92d1694832c418520ad487ecf4b20ad3a4))
* **deps:** update ubuntu:24.04 docker digest to 562111b ([#376](https://github.com/agentic-software-engineers/o3s/issues/376)) ([bc62010](https://github.com/agentic-software-engineers/o3s/commit/bc620107ed523663e5a6cc13a743ef8d64c46a78))

## [0.8.2](https://github.com/hansehart/o3s/compare/v0.8.1...v0.8.2) (2026-11-19)


### ⚠ BREAKING CHANGES

* **cage:** drop the docker socket shared with the host ([#312](https://github.com/hansehart/o3s/issues/312))

### Features

* **cage:** drop the docker socket shared with the host ([#312](https://github.com/hansehart/o3s/issues/312)) ([4f037d5](https://github.com/hansehart/o3s/commit/4f037d512b93b0e77818b1f3619beedab03579c8))
* **cage:** reach the gateway without NET_ADMIN ([#309](https://github.com/hansehart/o3s/issues/309)) ([3420ee7](https://github.com/hansehart/o3s/commit/3420ee74a6b8d95dbe13772f4247a9a89624f794))
* **cli:** show a feature's options in one view, one line per offer ([#303](https://github.com/hansehart/o3s/issues/303)) ([88d7e9a](https://github.com/hansehart/o3s/commit/88d7e9aaba3309fe7532fa9a4ea5b78d1dd69030))
* **gateway:** answer on the cage network's route address ([#306](https://github.com/hansehart/o3s/issues/306)) ([ab367c2](https://github.com/hansehart/o3s/commit/ab367c27604227959730524fc0ffe7cec57fd52e))


### Bug Fixes

* **cage:** close the profile's procfs and thermal throttle gaps ([#313](https://github.com/hansehart/o3s/issues/313)) ([15bc47c](https://github.com/hansehart/o3s/commit/15bc47c44eae7539f89e4fabe6573f5fd5a66864))
* **cli:** polish the firewall, feature, cage, init and update flows ([#302](https://github.com/hansehart/o3s/issues/302)) ([02b5fd0](https://github.com/hansehart/o3s/commit/02b5fd02731d613e27639b3201989f21dd834364))
* **cli:** round out the feature, store, cage, project and init dialogs ([#315](https://github.com/hansehart/o3s/issues/315)) ([c31bdbe](https://github.com/hansehart/o3s/commit/c31bdbe328ac6ff2212caf151631ef9bf9252c02))
* **cli:** round out the firewall and secret dialogs ([#314](https://github.com/hansehart/o3s/issues/314)) ([6f0b957](https://github.com/hansehart/o3s/commit/6f0b957aacf836214fea0267084e5f44f4006b4b))
* **cli:** settle how the menu behaves around a change ([#318](https://github.com/hansehart/o3s/issues/318)) ([3a70182](https://github.com/hansehart/o3s/commit/3a701826259fbf59b918f3d8b907f1730fada975))
* **deps:** update ubuntu:24.04 docker digest to 73b9993 ([#299](https://github.com/hansehart/o3s/issues/299)) ([9eb4453](https://github.com/hansehart/o3s/commit/9eb44531dc35ed53a29d6cf0cf10d7717da69816))

## [0.8.1](https://github.com/hansehart/o3s/compare/v0.8.0...v0.8.1) (2026-11-19)


### Bug Fixes

* **cli:** make init and the home's VM read clearly, and label the views ([#297](https://github.com/hansehart/o3s/issues/297)) ([43fccd5](https://github.com/hansehart/o3s/commit/43fccd5354cd6d28c89b7fd9422fceb65bc241d6))

## [0.8.0](https://github.com/hansehart/o3s/compare/v0.8.0-rc.1...v0.8.0) (2026-11-19)


### ⚠ BREAKING CHANGES

* **cli:** make the home inside its VM by default and run it from cage ([#294](https://github.com/hansehart/o3s/issues/294))

### Features

* **cli:** make the home inside its VM by default and run it from cage ([#294](https://github.com/hansehart/o3s/issues/294)) ([e536e7e](https://github.com/hansehart/o3s/commit/e536e7e59847938791f2865254d4740110cc8e22))

## [0.8.0-rc.1](https://github.com/hansehart/o3s/compare/v0.7.6-rc.2...v0.8.0-rc.1) (2026-11-19)


### ⚠ BREAKING CHANGES

* **cli:** open every group as a view and act on several names at once ([#292](https://github.com/hansehart/o3s/issues/292))

### Features

* **cli:** make the home inside its VM with o3s init --preview ([#264](https://github.com/hansehart/o3s/issues/264)) ([c7b342c](https://github.com/hansehart/o3s/commit/c7b342c74d3f264406a5985e7f5d5fdd3b904460))
* **cli:** open every group as a view and act on several names at once ([#292](https://github.com/hansehart/o3s/issues/292)) ([5bed122](https://github.com/hansehart/o3s/commit/5bed122976cd6c047f70024bf42e334ab4471b95))
* **repo:** install o3s and Lima on Linux and macOS with install.sh ([#268](https://github.com/hansehart/o3s/issues/268)) ([57c4f20](https://github.com/hansehart/o3s/commit/57c4f203b78da3d67f3fd87cf3b1a1f2120dbf2d))


### Bug Fixes

* **deps:** update module charm.land/lipgloss/v2 to v2.1.1 ([#288](https://github.com/hansehart/o3s/issues/288)) ([c1a3561](https://github.com/hansehart/o3s/commit/c1a35618ef267938a11bf544486b3e17693ac7cb))
* **deps:** update module github.com/klauspost/compress to v1.20.2 [security] ([#290](https://github.com/hansehart/o3s/issues/290)) ([205f47b](https://github.com/hansehart/o3s/commit/205f47b9c6918d3880a342c5f219e420d846f582))
* **repo:** release every dependency update that ships ([#287](https://github.com/hansehart/o3s/issues/287)) ([0450385](https://github.com/hansehart/o3s/commit/0450385a0785ee4f28725daa895536d3e40c8438))


### Code Refactoring

* **cli:** word the hosts, secrets and counts from one source ([#293](https://github.com/hansehart/o3s/issues/293)) ([00d5951](https://github.com/hansehart/o3s/commit/00d59510cd50fb14bcb7934e84ea2942912b5fcb))

## [0.7.6-rc.2](https://github.com/hansehart/o3s/compare/v0.7.6-rc.1...v0.7.6-rc.2) (2026-11-18)


### Features

* **cli:** set a feature's options when adding it ([#277](https://github.com/hansehart/o3s/issues/277)) ([69c8cfe](https://github.com/hansehart/o3s/commit/69c8cfef43f8892bee4f1c1b7ec145d1a694a087))


### Bug Fixes

* **cli:** keep the confirmation in view beside a summary ([#283](https://github.com/hansehart/o3s/issues/283)) ([f7b8377](https://github.com/hansehart/o3s/commit/f7b83779d9d37428dbb42f0ec1fd0a560a6ab872))
* **cli:** keep the menu's output whole and its failures named ([#275](https://github.com/hansehart/o3s/issues/275)) ([e26dd31](https://github.com/hansehart/o3s/commit/e26dd31acfa8e47c14ff4525bec385c7dbc060d5))
* **cli:** open o3s cage ide over SSH ([#285](https://github.com/hansehart/o3s/issues/285)) ([fbfe0eb](https://github.com/hansehart/o3s/commit/fbfe0eb71266f80ea59958c5aeb11e13bbcb9d9b))
* **repo:** decide installs and updates by the checksum alone ([#267](https://github.com/hansehart/o3s/issues/267)) ([2cf8d0b](https://github.com/hansehart/o3s/commit/2cf8d0bd2dad7e04ecf82a985d87822afe4fb295))

## [0.7.6-rc.1](https://github.com/hansehart/o3s/compare/v0.7.5...v0.7.6-rc.1) (2026-11-16)


### Features

* **cli:** add, remove and list projects with o3s project ([#256](https://github.com/hansehart/o3s/issues/256)) ([fa6ea6c](https://github.com/hansehart/o3s/commit/fa6ea6cd8dff7aae3fa30f389856915339314eb7))
* **cli:** lay out projects by host, owner and name ([#258](https://github.com/hansehart/o3s/issues/258)) ([99ea433](https://github.com/hansehart/o3s/commit/99ea4337851c05559e3c0220132b65f7e2d52263))
* **cli:** run the home's VM with o3s vm start, shell, stop, rm ([#259](https://github.com/hansehart/o3s/issues/259)) ([05f055e](https://github.com/hansehart/o3s/commit/05f055e71448b726e46d9eab8cbb9f6aa2f4cb0a))
* **cli:** update each binary within its channel ([#248](https://github.com/hansehart/o3s/issues/248)) ([4c732a3](https://github.com/hansehart/o3s/commit/4c732a3b69415dd9532009a0bbcb97d49eb2302d))
* **repo:** let allow rules expire after a set time ([#265](https://github.com/hansehart/o3s/issues/265)) ([b1c722a](https://github.com/hansehart/o3s/commit/b1c722a156b0619cae3d4d7d3455dd94c0fd25c4))
* **repo:** release every merge to main as a release candidate ([#245](https://github.com/hansehart/o3s/issues/245)) ([91e4897](https://github.com/hansehart/o3s/commit/91e48974f7a98a51015b10b2c2aefe2a59654787))
* **repo:** show each host's traffic and the names the gateway blocked ([#262](https://github.com/hansehart/o3s/issues/262)) ([53114bd](https://github.com/hansehart/o3s/commit/53114bdb5b857a4e674d796c135139819135405f))


### Bug Fixes

* **cli:** point the AppArmor warning at the secure default ([#261](https://github.com/hansehart/o3s/issues/261)) ([82e428f](https://github.com/hansehart/o3s/commit/82e428f52fb7869e110830fe370f8fedc615537a))
* **cli:** refuse a cage without AppArmor unless the home opts out ([#252](https://github.com/hansehart/o3s/issues/252)) ([dc9347a](https://github.com/hansehart/o3s/commit/dc9347a918770aefe95c37ba5b0d9f072e33b2b2))
* **repo:** publish a release once its files are in place ([#251](https://github.com/hansehart/o3s/issues/251)) ([2a66c9e](https://github.com/hansehart/o3s/commit/2a66c9ef0755aeb7b4362e9e5638232d1cff836f))

## [0.7.5](https://github.com/hansehart/o3s/compare/v0.7.4...v0.7.5) (2026-11-15)


### Features

* **cli:** draw the menu as one app ([#241](https://github.com/hansehart/o3s/issues/241)) ([0e37d9e](https://github.com/hansehart/o3s/commit/0e37d9ec06603d1a2527b95965b7694880c5bc63))

## [0.7.4](https://github.com/hansehart/o3s/compare/v0.7.3...v0.7.4) (2026-11-13)


### Features

* **cli:** pin the stack's images by digest ([#233](https://github.com/hansehart/o3s/issues/233)) ([eb0dfff](https://github.com/hansehart/o3s/commit/eb0dfff1070fa8354bb291b5f2e6f2605723b05d))


### Bug Fixes

* **cli:** pull the gateway once per version ([#232](https://github.com/hansehart/o3s/issues/232)) ([f315bd2](https://github.com/hansehart/o3s/commit/f315bd2bc99340e5c9a8aebddabdc8ecd1e7bf33))

## [0.7.3](https://github.com/hansehart/o3s/compare/v0.7.2...v0.7.3) (2026-11-13)


### Features

* **cli:** keep the menu open in its own screen ([#230](https://github.com/hansehart/o3s/issues/230)) ([fbb2a7a](https://github.com/hansehart/o3s/commit/fbb2a7ab608819dc0381294f8e1d536e399d892e))

## [0.7.2](https://github.com/hansehart/o3s/compare/v0.7.1...v0.7.2) (2026-11-13)


### Features

* **cli:** add several features at once ([#228](https://github.com/hansehart/o3s/issues/228)) ([5a3b6b7](https://github.com/hansehart/o3s/commit/5a3b6b7be29fb4921fe6390233c9853bcc6ff42e))

## [0.7.1](https://github.com/hansehart/o3s/compare/v0.7.0...v0.7.1) (2026-11-13)


### Bug Fixes

* **repo:** install the devcontainer CLI on a bare home ([#226](https://github.com/hansehart/o3s/issues/226)) ([fbd4662](https://github.com/hansehart/o3s/commit/fbd466258688b282078189feadcd3a6334600047))

## [0.7.0](https://github.com/hansehart/o3s/compare/v0.6.6...v0.7.0) (2026-11-13)


### Features

* **gateway:** read the home's policy when it is mounted ([#183](https://github.com/hansehart/o3s/issues/183)) ([ba86b23](https://github.com/hansehart/o3s/commit/ba86b230b3f7f61e4176df9b76e1af01db4a6dbc))
* **repo:** allow, remove and list hosts with o3s firewall ([#182](https://github.com/hansehart/o3s/issues/182)) ([5d1ea5b](https://github.com/hansehart/o3s/commit/5d1ea5bb40eeb8fd10bf89eea0d42b62dbf18f7d))
* **repo:** bring a home up to date with o3s update ([#195](https://github.com/hansehart/o3s/issues/195)) ([6d7e883](https://github.com/hansehart/o3s/commit/6d7e883e385547e5b5ead4d32f620f9df6ae7dc8))
* **repo:** install o3s with curl and a checked script ([#204](https://github.com/hansehart/o3s/issues/204)) ([ee0067f](https://github.com/hansehart/o3s/commit/ee0067f54b274a6fbff8f093be1a32d6089b45d1))
* **repo:** keep what o3s makes in the XDG folders ([#209](https://github.com/hansehart/o3s/issues/209)) ([588d0e6](https://github.com/hansehart/o3s/commit/588d0e6ed637c2a45a37acb0f0ebc774e3a29d06))
* **repo:** lead through every dialog with a menu on bare o3s ([#196](https://github.com/hansehart/o3s/issues/196)) ([6e47924](https://github.com/hansehart/o3s/commit/6e47924d835edc85821c870dbea33a2e680e333d))
* **repo:** lead through the feature dialogs in the menu ([#199](https://github.com/hansehart/o3s/issues/199)) ([d91da69](https://github.com/hansehart/o3s/commit/d91da699c43f71786e58c43c9a2076b7224a9a3d))
* **repo:** make a folder into a home with o3s init ([#189](https://github.com/hansehart/o3s/issues/189)) ([50a2263](https://github.com/hansehart/o3s/commit/50a226374200aba6e71a3f3a3dcb15a1958d8056))
* **repo:** manage the cage's features with o3s feature ([#194](https://github.com/hansehart/o3s/issues/194)) ([3266c8d](https://github.com/hansehart/o3s/commit/3266c8d826cd693a65e3a21301ecdaff25c5cdd0))
* **repo:** manage the feature stores with o3s store ([#193](https://github.com/hansehart/o3s/issues/193)) ([ae25f84](https://github.com/hansehart/o3s/commit/ae25f8456d56249390e1afb24276b31bf7f740c3))
* **repo:** open the cage in VS Code or a shell ([#221](https://github.com/hansehart/o3s/issues/221)) ([5ff3793](https://github.com/hansehart/o3s/commit/5ff37938bc443b715e30eb54f7a47b3baa084d87))
* **repo:** prepare the host with o3s init on every start ([#208](https://github.com/hansehart/o3s/issues/208)) ([e2e7305](https://github.com/hansehart/o3s/commit/e2e73056c045795972140635aec65025509a831b))
* **repo:** publish the o3s binaries with every release ([#200](https://github.com/hansehart/o3s/issues/200)) ([c3e1b7c](https://github.com/hansehart/o3s/commit/c3e1b7cd7aa05ef665104a24feddc0aeb3c6d2af))
* **repo:** run the stack on the home's o3s.json ([#187](https://github.com/hansehart/o3s/issues/187)) ([80d38ea](https://github.com/hansehart/o3s/commit/80d38eaefd757c8c1093d67832f0f34a0271875b))
* **repo:** ship the schema of o3s.json with every home ([#191](https://github.com/hansehart/o3s/issues/191)) ([22981ef](https://github.com/hansehart/o3s/commit/22981efb5869ce42eb88250d391c83cd204b0ac0))
* **repo:** store, remove and list secrets with o3s secret ([#188](https://github.com/hansehart/o3s/issues/188)) ([0429977](https://github.com/hansehart/o3s/commit/042997728bae16d2bb90b1b27fb4616782a5819d))
* **repo:** update the binary and rebuild the cage with o3s update ([#205](https://github.com/hansehart/o3s/issues/205)) ([ba9e383](https://github.com/hansehart/o3s/commit/ba9e383bc254e7d231f828fd1ec6d2f4ac4d2325))


### Bug Fixes

* **gateway:** start on an empty policy ([#179](https://github.com/hansehart/o3s/issues/179)) ([0ceedb5](https://github.com/hansehart/o3s/commit/0ceedb57da0f051149c50e316225b725bd4cd713))
* **repo:** hold every entry of o3s.json to its rules on load ([#192](https://github.com/hansehart/o3s/issues/192)) ([9afa168](https://github.com/hansehart/o3s/commit/9afa168602dabe8860d7d1a9f32018c2d8a59ed9))
* **repo:** keep the host's secrets out of a home's cage ([#212](https://github.com/hansehart/o3s/issues/212)) ([bb58afa](https://github.com/hansehart/o3s/commit/bb58afa20f850a7848ba3453dbdb709f6861d57d))
* **repo:** point a new home at o3s cage ide ([#224](https://github.com/hansehart/o3s/issues/224)) ([f58bc5e](https://github.com/hansehart/o3s/commit/f58bc5eef5093018d0786121e652755ba862bfb6))
* **repo:** smooth the menu's output and the host rule ([#213](https://github.com/hansehart/o3s/issues/213)) ([37b95aa](https://github.com/hansehart/o3s/commit/37b95aae1c9c9e891b732b60887a571fadb00eec))


### Reverts

* **repo:** take the release scope back out ([c0678c3](https://github.com/hansehart/o3s/commit/c0678c39da42ff65702b4c20702ab1db3980b94a))
