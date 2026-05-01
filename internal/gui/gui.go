package gui

import (
	"fmt"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"emusave.jey/internal/appconfig"
	"emusave.jey/internal/backup"
	"emusave.jey/internal/ryu"
	"emusave.jey/internal/scanner"
	"emusave.jey/internal/smbclient"
	"emusave.jey/internal/util"
)

type state struct {
	cfg         *appconfig.Config
	games       []scanner.Game
	wizardIndex int
	currentGame int
	page        string

	statusText *widget.Entry
	mainWindow fyne.Window
	mainBody   *fyne.Container

	ryuCfgEntry  *widget.Entry
	ryuDataEntry *widget.Entry

	smbHostEntry   *widget.Entry
	smbShareEntry  *widget.Entry
	smbBaseEntry   *widget.Entry
	smbUserEntry   *widget.Entry
	smbPassEntry   *widget.Entry
	smbDomainEntry *widget.Entry

	localBaseEntry *widget.Entry
	commandEntry   *widget.Entry
	syncModeRadio  *widget.RadioGroup
}

func Run() {
	a := app.New()
	w := a.NewWindow("emusave.jey")
	w.Resize(fyne.NewSize(1180, 820))

	cfg, err := appconfig.Load()
	if err != nil {
		cfg = appconfig.DefaultConfig()
	}

	st := &state{
		cfg:        cfg,
		mainWindow: w,
		page:       "main",
	}
	st.initWidgets()
	st.prefillDefaults()

	if st.cfg.FirstRunCompleted {
		st.showMain()
	} else {
		st.showWizard()
	}

	w.ShowAndRun()
}

func (s *state) initWidgets() {
	s.statusText = widget.NewMultiLineEntry()
	s.statusText.Disable()
	s.statusText.SetMinRowsVisible(8)

	s.ryuCfgEntry = widget.NewEntry()
	s.ryuCfgEntry.SetPlaceHolder("~/.config/Ryujinx/Config.json")
	s.ryuCfgEntry.SetText(s.cfg.RyujinxConfigPath)

	s.ryuDataEntry = widget.NewEntry()
	s.ryuDataEntry.SetText(s.cfg.RyujinxDataRoot)

	s.smbHostEntry = widget.NewEntry()
	s.smbHostEntry.SetText(s.cfg.SMB.Host)

	s.smbShareEntry = widget.NewEntry()
	s.smbShareEntry.SetText(s.cfg.SMB.Share)

	s.smbBaseEntry = widget.NewEntry()
	s.smbBaseEntry.SetText(s.cfg.SMB.BaseDir)

	s.smbUserEntry = widget.NewEntry()
	s.smbUserEntry.SetText(s.cfg.SMB.Username)

	s.smbPassEntry = widget.NewPasswordEntry()
	s.smbPassEntry.SetText(util.DecodeBase64(s.cfg.SMB.PasswordBase64))

	s.smbDomainEntry = widget.NewEntry()
	s.smbDomainEntry.SetText(s.cfg.SMB.Domain)

	s.localBaseEntry = widget.NewEntry()
	s.localBaseEntry.SetText(s.cfg.Local.BasePath)

	s.commandEntry = widget.NewEntry()
	s.commandEntry.SetPlaceHolder("Example: rsync -a {save_path}/ /backup/{title_name}/{timestamp}/")
	s.commandEntry.SetText(s.cfg.Command.Command)

	s.syncModeRadio = widget.NewRadioGroup([]string{
		string(appconfig.SyncModeSMB),
		string(appconfig.SyncModeLocal),
		string(appconfig.SyncModeCommand),
	}, nil)
	s.syncModeRadio.SetSelected(string(s.cfg.SyncMode))
	if s.syncModeRadio.Selected == "" {
		s.syncModeRadio.SetSelected(string(appconfig.SyncModeSMB))
	}
}

func (s *state) prefillDefaults() {
	if strings.TrimSpace(s.ryuCfgEntry.Text) == "" {
		defaultCfg := util.DefaultRyujinxConfigPath()
		s.ryuCfgEntry.SetText(defaultCfg)
		s.cfg.RyujinxConfigPath = defaultCfg
	}

	if strings.TrimSpace(s.ryuDataEntry.Text) == "" {
		s.cfg.RyujinxDataRoot = ryu.GuessDefaultDataRoot()
		s.ryuDataEntry.SetText(s.cfg.RyujinxDataRoot)
	}
}

func (s *state) setStatus(lines ...string) {
	s.statusText.SetText(strings.Join(lines, "\n"))
}

func (s *state) saveConfigFromWidgets() error {
	s.cfg.RyujinxConfigPath = strings.TrimSpace(s.ryuCfgEntry.Text)
	s.cfg.RyujinxDataRoot = strings.TrimSpace(s.ryuDataEntry.Text)
	s.cfg.SyncMode = appconfig.SyncMode(s.syncModeRadio.Selected)

	s.cfg.SMB.Host = strings.TrimSpace(s.smbHostEntry.Text)
	s.cfg.SMB.Share = strings.TrimSpace(s.smbShareEntry.Text)
	s.cfg.SMB.BaseDir = strings.TrimSpace(s.smbBaseEntry.Text)
	s.cfg.SMB.Username = strings.TrimSpace(s.smbUserEntry.Text)
	s.cfg.SMB.PasswordBase64 = util.EncodeBase64(s.smbPassEntry.Text)
	s.cfg.SMB.Domain = strings.TrimSpace(s.smbDomainEntry.Text)

	s.cfg.Local.BasePath = strings.TrimSpace(s.localBaseEntry.Text)
	s.cfg.Command.Command = strings.TrimSpace(s.commandEntry.Text)

	return appconfig.Save(s.cfg)
}

func (s *state) validateCurrentMode() error {
	switch s.cfg.SyncMode {
	case appconfig.SyncModeSMB:
		if s.cfg.SMB.Host == "" {
			return fmt.Errorf("SMB IP/host is empty")
		}
		if s.cfg.SMB.Share == "" {
			return fmt.Errorf("SMB share is empty")
		}
		if s.cfg.SMB.BaseDir == "" {
			return fmt.Errorf("SMB path inside share is empty")
		}
		if s.cfg.SMB.Username == "" {
			return fmt.Errorf("SMB username is empty")
		}
		if util.DecodeBase64(s.cfg.SMB.PasswordBase64) == "" {
			return fmt.Errorf("SMB password is empty")
		}
	case appconfig.SyncModeLocal:
		if util.ExpandUserPath(s.cfg.Local.BasePath) == "" {
			return fmt.Errorf("local backup path is empty")
		}
	case appconfig.SyncModeCommand:
		if s.cfg.Command.Command == "" {
			return fmt.Errorf("custom command is empty")
		}
	default:
		return fmt.Errorf("invalid sync mode")
	}
	return nil
}

func (s *state) scanGamesAndCandidates() error {
	if strings.TrimSpace(s.cfg.RyujinxConfigPath) == "" {
		return fmt.Errorf("Ryujinx config path is empty")
	}

	rc, err := ryu.ParseConfig(s.cfg.RyujinxConfigPath)
	if err != nil {
		return fmt.Errorf("load Ryujinx config: %w", err)
	}
	if len(rc.GameDirs) == 0 {
		return fmt.Errorf("Ryujinx config has no game_dirs")
	}
	s.cfg.GameDirs = rc.GameDirs

	games, err := scanner.ScanGameFiles(rc.GameDirs)
	if err != nil {
		return fmt.Errorf("scan game files: %w", err)
	}
	if len(games) == 0 {
		return fmt.Errorf("no .nsp or .xci files found in configured game_dirs")
	}

	saveRoot := ryu.SaveRoot(s.cfg.RyujinxDataRoot)
	for i := range games {
		cands, _ := ryu.DetectSaveCandidatesForGame(saveRoot, games[i].TitleID, games[i].ROMPath)
		games[i].SaveOptions = cands

		if gm, ok := s.cfg.Games[games[i].Key]; ok {
			if gm.TitleName != "" {
				games[i].TitleName = gm.TitleName
			}
			if gm.TitleID != "" {
				games[i].TitleID = gm.TitleID
			}
			if gm.ConfirmedSavePath != "" {
				games[i].SavePath = gm.ConfirmedSavePath
			}
			if len(games[i].SaveOptions) == 0 && len(gm.DetectedCandidates) > 0 {
				games[i].SaveOptions = append([]string{}, gm.DetectedCandidates...)
			}
		}

		if games[i].SavePath == "" && len(games[i].SaveOptions) > 0 {
			games[i].SavePath = games[i].SaveOptions[0]
		}
	}

	s.games = games
	return appconfig.Save(s.cfg)
}

func (s *state) showWizard() {
	s.wizardIndex = 0
	s.currentGame = 0
	s.renderWizardPage()
}

func (s *state) renderWizardPage() {
	var content fyne.CanvasObject

	switch s.wizardIndex {
	case 0:
		content = s.pageWelcome()
	case 1:
		content = s.pageRyujinxConfig()
	case 2:
		content = s.pageSyncTarget()
	case 3:
		content = s.pageSaveDetection()
	default:
		s.cfg.FirstRunCompleted = true
		_ = appconfig.Save(s.cfg)
		s.showMain()
		return
	}

	s.mainWindow.SetContent(container.NewBorder(
		widget.NewLabel("emusave.jey setup"),
		nil,
		nil,
		nil,
		content,
	))
}

func (s *state) pageWelcome() fyne.CanvasObject {
	nextBtn := widget.NewButton("Next", func() {
		s.wizardIndex = 1
		s.renderWizardPage()
	})

	box := container.NewVBox(
		widget.NewLabel("Hi, let's set this up."),
		widget.NewLabel("This wizard will ask for your Ryujinx config, your sync target, and then help confirm save locations."),
		layout.NewSpacer(),
		nextBtn,
	)
	return container.NewPadded(box)
}

func (s *state) chooseRyujinxConfigFile() {
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()

		s.ryuCfgEntry.SetText(reader.URI().Path())
	}, s.mainWindow)

	fd.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
	fd.Show()
}

func (s *state) chooseLocalFolder() {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}
		if uri == nil {
			return
		}
		s.localBaseEntry.SetText(uri.Path())
	}, s.mainWindow)
}

func (s *state) browseSMBPath() {
	if err := s.saveConfigFromWidgets(); err != nil {
		dialog.ShowError(err, s.mainWindow)
		return
	}

	if strings.TrimSpace(s.smbHostEntry.Text) == "" {
		dialog.ShowError(fmt.Errorf("SMB IP/host is empty"), s.mainWindow)
		return
	}
	if strings.TrimSpace(s.smbUserEntry.Text) == "" {
		dialog.ShowError(fmt.Errorf("SMB username is empty"), s.mainWindow)
		return
	}
	if strings.TrimSpace(s.smbPassEntry.Text) == "" {
		dialog.ShowError(fmt.Errorf("SMB password is empty"), s.mainWindow)
		return
	}
	if strings.TrimSpace(s.smbShareEntry.Text) == "" {
		dialog.ShowError(fmt.Errorf("SMB share is empty"), s.mainWindow)
		return
	}

	client, err := smbclient.Connect(s.cfg.SMB, util.DecodeBase64(s.cfg.SMB.PasswordBase64))
	if err != nil {
		dialog.ShowError(err, s.mainWindow)
		return
	}
	defer client.Close()

	current := strings.TrimSpace(s.smbBaseEntry.Text)
	s.showSMBFolderBrowser(client, current)
}

func (s *state) showSMBFolderBrowser(client *smbclient.Client, startPath string) {
	current := strings.Trim(startPath, "/")

	var win fyne.Window
	if app := fyne.CurrentApp(); app != nil {
		win = app.NewWindow("Browse SMB path")
	} else {
		dialog.ShowError(fmt.Errorf("cannot create SMB browser window"), s.mainWindow)
		return
	}
	win.Resize(fyne.NewSize(700, 500))

	pathLabel := widget.NewLabel("")
	listBox := container.NewVBox()
	scroll := container.NewVScroll(listBox)

	var refresh func()

	refresh = func() {
		displayPath := current
		if displayPath == "" {
			displayPath = "/"
		}
		pathLabel.SetText("Current path: " + displayPath)

		listBox.Objects = nil

		if current != "" {
			upBtn := widget.NewButton("..", func() {
				parts := strings.Split(strings.Trim(current, "/"), "/")
				if len(parts) <= 1 {
					current = ""
				} else {
					current = strings.Join(parts[:len(parts)-1], "/")
				}
				refresh()
			})
			listBox.Add(upBtn)
		}

		entries, err := client.ListDir(current)
		if err != nil {
			listBox.Add(widget.NewLabel("Error: " + err.Error()))
			listBox.Refresh()
			return
		}

		if len(entries) == 0 {
			listBox.Add(widget.NewLabel("(empty folder)"))
		}

		for _, entry := range entries {
			e := entry
			btn := widget.NewButton("📁 "+e.Name, func() {
				current = e.Path
				refresh()
			})
			listBox.Add(btn)
		}

		listBox.Refresh()
	}

	selectBtn := widget.NewButton("Use this path", func() {
		s.smbBaseEntry.SetText(current)
		_ = s.saveConfigFromWidgets()
		win.Close()
	})

	rootBtn := widget.NewButton("Use share root", func() {
		s.smbBaseEntry.SetText("")
		_ = s.saveConfigFromWidgets()
		win.Close()
	})

	refresh()
	win.SetContent(container.NewBorder(
		container.NewVBox(pathLabel),
		container.NewHBox(rootBtn, selectBtn),
		nil,
		nil,
		scroll,
	))
	win.Show()
}

func (s *state) pageRyujinxConfig() fyne.CanvasObject {
	info := widget.NewLabel("First, select your Ryujinx config.json. The default on Linux is ~/.config/Ryujinx/Config.json.")

	autoBtn := widget.NewButton("Use default path", func() {
		s.ryuCfgEntry.SetText(util.DefaultRyujinxConfigPath())
	})

	pickBtn := widget.NewButton("Choose config.json", func() {
		s.chooseRyujinxConfigFile()
	})

	checkBtn := widget.NewButton("Validate + continue", func() {
		if err := s.saveConfigFromWidgets(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}

		if strings.TrimSpace(s.cfg.RyujinxConfigPath) == "" {
			dialog.ShowError(fmt.Errorf("please enter your Ryujinx config.json path"), s.mainWindow)
			return
		}
		if _, err := os.Stat(s.cfg.RyujinxConfigPath); err != nil {
			dialog.ShowError(fmt.Errorf("Ryujinx config.json not found: %w", err), s.mainWindow)
			return
		}

		rc, err := ryu.ParseConfig(s.cfg.RyujinxConfigPath)
		if err != nil {
			dialog.ShowError(fmt.Errorf("failed to parse Ryujinx config: %w", err), s.mainWindow)
			return
		}
		if len(rc.GameDirs) == 0 {
			dialog.ShowError(fmt.Errorf("config.json was loaded, but game_dirs is empty"), s.mainWindow)
			return
		}

		s.cfg.GameDirs = rc.GameDirs
		if err := s.saveConfigFromWidgets(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}

		s.wizardIndex = 2
		s.renderWizardPage()
	})

	backBtn := widget.NewButton("Back", func() {
		s.wizardIndex = 0
		s.renderWizardPage()
	})

	return container.NewPadded(container.NewVBox(
		info,
		widget.NewForm(
			widget.NewFormItem("Ryujinx config.json", s.ryuCfgEntry),
			widget.NewFormItem("Ryujinx data root", s.ryuDataEntry),
		),
		container.NewHBox(autoBtn, pickBtn),
		container.NewHBox(backBtn, checkBtn),
	))
}

func (s *state) pageSyncTarget() fyne.CanvasObject {
	info := widget.NewLabel("Where do you want to sync your games?")

	modeForm := container.NewVBox(
		widget.NewLabel("Choose sync mode"),
		s.syncModeRadio,
	)

	smbForm := widget.NewForm(
		widget.NewFormItem("IP / Host", s.smbHostEntry),
		widget.NewFormItem("Username", s.smbUserEntry),
		widget.NewFormItem("Password", s.smbPassEntry),
		widget.NewFormItem("Domain (optional)", s.smbDomainEntry),
		widget.NewFormItem("Share", s.smbShareEntry),
		widget.NewFormItem("Path in share", s.smbBaseEntry),
	)

	localForm := widget.NewForm(
		widget.NewFormItem("Local base path", s.localBaseEntry),
	)

	commandForm := widget.NewForm(
		widget.NewFormItem("Custom command", s.commandEntry),
	)

	browseSMBBtn := widget.NewButton("Browse SMB path", func() {
		s.browseSMBPath()
	})

	pickLocalBtn := widget.NewButton("Choose local folder", func() {
		s.chooseLocalFolder()
	})

	testBtn := widget.NewButton("Test selected target", func() {
		if err := s.saveConfigFromWidgets(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}
		if err := s.validateCurrentMode(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}

		switch s.cfg.SyncMode {
		case appconfig.SyncModeSMB:
			client, err := smbclient.Connect(s.cfg.SMB, util.DecodeBase64(s.cfg.SMB.PasswordBase64))
			if err != nil {
				dialog.ShowError(err, s.mainWindow)
				return
			}
			defer client.Close()

			if err := client.TestWrite(s.cfg.SMB.BaseDir); err != nil {
				dialog.ShowError(err, s.mainWindow)
				return
			}
			dialog.ShowInformation("Success", "SMB login and test write worked.", s.mainWindow)

		case appconfig.SyncModeLocal:
			p := util.ExpandUserPath(s.cfg.Local.BasePath)
			if p == "" {
				dialog.ShowError(fmt.Errorf("local path is empty"), s.mainWindow)
				return
			}
			if err := os.MkdirAll(p, 0o755); err != nil {
				dialog.ShowError(fmt.Errorf("cannot create local base path: %w", err), s.mainWindow)
				return
			}
			dialog.ShowInformation("Success", "Local backup path is usable.", s.mainWindow)

		case appconfig.SyncModeCommand:
			dialog.ShowInformation("Command mode", "Command saved. It will run during backup with placeholders.", s.mainWindow)
		}
	})

	nextBtn := widget.NewButton("Continue", func() {
		if err := s.saveConfigFromWidgets(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}
		if err := s.validateCurrentMode(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}
		if err := s.scanGamesAndCandidates(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}

		s.currentGame = 0
		s.wizardIndex = 3
		s.renderWizardPage()
	})

	backBtn := widget.NewButton("Back", func() {
		s.wizardIndex = 1
		s.renderWizardPage()
	})

	return container.NewPadded(container.NewVBox(
		info,
		modeForm,
		widget.NewSeparator(),
		widget.NewLabel("SMB share"),
		smbForm,
		container.NewHBox(browseSMBBtn),
		widget.NewSeparator(),
		widget.NewLabel("Custom location"),
		localForm,
		container.NewHBox(pickLocalBtn),
		widget.NewSeparator(),
		widget.NewLabel("Custom command"),
		commandForm,
		widget.NewLabel("Placeholders: {save_path} {title_name} {title_id} {rom_path} {timestamp}"),
		container.NewHBox(backBtn, testBtn, nextBtn),
	))
}

func (s *state) pageSaveDetection() fyne.CanvasObject {
	if len(s.games) == 0 {
		return container.NewPadded(container.NewVBox(
			widget.NewLabel("No games found."),
			widget.NewButton("Back", func() {
				s.wizardIndex = 2
				s.renderWizardPage()
			}),
		))
	}

	if s.currentGame >= len(s.games) {
		s.cfg.FirstRunCompleted = true
		for _, g := range s.games {
			m := s.cfg.Games[g.Key]
			m.TitleID = g.TitleID
			m.TitleName = g.TitleName
			m.ROMPath = g.ROMPath
			m.ConfirmedSavePath = g.SavePath
			m.DetectedCandidates = append([]string{}, g.SaveOptions...)
			s.cfg.Games[g.Key] = m
		}
		_ = appconfig.Save(s.cfg)
		s.showMain()
		return nil
	}

	g := &s.games[s.currentGame]
	titleNameEntry := widget.NewEntry()
	titleNameEntry.SetText(g.TitleName)

	titleIDEntry := widget.NewEntry()
	titleIDEntry.SetText(g.TitleID)

	savePathEntry := widget.NewEntry()
	savePathEntry.SetText(g.SavePath)

	candidateLabel := widget.NewLabel(strings.Join(g.SaveOptions, "\n"))
	if len(g.SaveOptions) == 0 {
		candidateLabel.SetText("No candidates found automatically. You need to enter the path manually.")
	}

	nextBtn := widget.NewButton("This is correct, next game", func() {
		g.TitleName = strings.TrimSpace(titleNameEntry.Text)
		g.TitleID = strings.ToUpper(strings.TrimSpace(titleIDEntry.Text))
		g.SavePath = strings.TrimSpace(savePathEntry.Text)

		if g.SavePath == "" {
			dialog.ShowError(fmt.Errorf("save path is empty"), s.mainWindow)
			return
		}
		if st, err := os.Stat(g.SavePath); err != nil || !st.IsDir() {
			dialog.ShowError(fmt.Errorf("save path does not exist or is not a directory: %s", g.SavePath), s.mainWindow)
			return
		}

		m := s.cfg.Games[g.Key]
		m.TitleID = g.TitleID
		m.TitleName = g.TitleName
		m.ROMPath = g.ROMPath
		m.ConfirmedSavePath = g.SavePath
		m.DetectedCandidates = append([]string{}, g.SaveOptions...)
		s.cfg.Games[g.Key] = m
		_ = appconfig.Save(s.cfg)

		s.currentGame++
		s.renderWizardPage()
	})

	backBtn := widget.NewButton("Back", func() {
		if s.currentGame == 0 {
			s.wizardIndex = 2
		} else {
			s.currentGame--
		}
		s.renderWizardPage()
	})

	skipBtn := widget.NewButton("Skip this game for now", func() {
		s.currentGame++
		s.renderWizardPage()
	})

	header := widget.NewLabel(fmt.Sprintf("Let's try to figure out the right save locations... (%d/%d)", s.currentGame+1, len(s.games)))

	return container.NewPadded(container.NewVBox(
		header,
		widget.NewLabel("Game ROM"),
		widget.NewLabel(g.ROMPath),
		widget.NewForm(
			widget.NewFormItem("Game name", titleNameEntry),
			widget.NewFormItem("Title ID", titleIDEntry),
			widget.NewFormItem("Save path", savePathEntry),
		),
		widget.NewLabel("Detected candidate paths"),
		candidateLabel,
		container.NewHBox(backBtn, skipBtn, nextBtn),
	))
}

func (s *state) showMain() {
	s.page = "main"
	s.renderMain()
	s.mainWindow.SetContent(s.mainBody)
}

func (s *state) showSettings() {
	s.page = "settings"
	s.renderSettings()
	s.mainWindow.SetContent(s.mainBody)
}

func (s *state) renderMain() {
	refreshBtn := widget.NewButton("Rescan games", func() {
		if err := s.saveConfigFromWidgets(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}
		if err := s.scanGamesAndCandidates(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}
		s.renderMain()
		s.mainWindow.SetContent(s.mainBody)
		s.setStatus("Rescan finished.", fmt.Sprintf("Games found: %d", len(s.games)))
	})

	settingsBtn := widget.NewButton("Settings", func() {
		s.showSettings()
	})

	backupAllBtn := widget.NewButton("Backup all mapped games", func() {
		if err := s.saveConfigFromWidgets(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}

		var lines []string
		for i := range s.games {
			g := &s.games[i]
			if strings.TrimSpace(g.SavePath) == "" {
				continue
			}

			target, err := backup.Run(s.cfg, *g)
			if err != nil {
				dialog.ShowError(fmt.Errorf("%s: %w", g.TitleName, err), s.mainWindow)
				return
			}
			_ = appconfig.Save(s.cfg)
			lines = append(lines, g.TitleName+" -> "+target)
		}

		if len(lines) == 0 {
			lines = append(lines, "No mapped games to back up.")
		}
		s.setStatus(append([]string{"Backup finished:"}, lines...)...)
		s.renderMain()
		s.mainWindow.SetContent(s.mainBody)
	})

	top := container.NewHBox(
		refreshBtn,
		backupAllBtn,
		settingsBtn,
		layout.NewSpacer(),
	)

	gameList := container.NewVBox()
	if len(s.games) == 0 {
		gameList.Add(widget.NewLabel("No games loaded yet. Click Rescan games."))
	} else {
		for i := range s.games {
			g := &s.games[i]

			titleEntry := widget.NewEntry()
			titleEntry.SetText(g.TitleName)

			titleIDEntry := widget.NewEntry()
			titleIDEntry.SetText(g.TitleID)

			saveEntry := widget.NewEntry()
			saveEntry.SetText(g.SavePath)

			candidates := widget.NewLabel(strings.Join(g.SaveOptions, "\n"))
			if len(g.SaveOptions) == 0 {
				candidates.SetText("No automatic candidates found")
			}

			backupBtn := widget.NewButton("Backup this game", func(game *scanner.Game, tE, idE, sE *widget.Entry) func() {
				return func() {
					game.TitleName = strings.TrimSpace(tE.Text)
					game.TitleID = strings.ToUpper(strings.TrimSpace(idE.Text))
					game.SavePath = strings.TrimSpace(sE.Text)

					target, err := backup.Run(s.cfg, *game)
					if err != nil {
						dialog.ShowError(err, s.mainWindow)
						return
					}
					_ = appconfig.Save(s.cfg)
					s.setStatus("Backup finished:", game.TitleName, target)
					s.renderMain()
					s.mainWindow.SetContent(s.mainBody)
				}
			}(g, titleEntry, titleIDEntry, saveEntry))

			saveMapBtn := widget.NewButton("Save mapping", func(game *scanner.Game, tE, idE, sE *widget.Entry) func() {
				return func() {
					game.TitleName = strings.TrimSpace(tE.Text)
					game.TitleID = strings.ToUpper(strings.TrimSpace(idE.Text))
					game.SavePath = strings.TrimSpace(sE.Text)

					m := s.cfg.Games[game.Key]
					m.TitleID = game.TitleID
					m.TitleName = game.TitleName
					m.ROMPath = game.ROMPath
					m.ConfirmedSavePath = game.SavePath
					m.DetectedCandidates = append([]string{}, game.SaveOptions...)
					s.cfg.Games[game.Key] = m

					if err := appconfig.Save(s.cfg); err != nil {
						dialog.ShowError(err, s.mainWindow)
						return
					}
					s.setStatus("Saved mapping for", game.TitleName)
				}
			}(g, titleEntry, titleIDEntry, saveEntry))

			card := widget.NewCard(
				g.TitleName,
				g.ROMPath,
				container.NewVBox(
					widget.NewForm(
						widget.NewFormItem("Game name", titleEntry),
						widget.NewFormItem("Title ID", titleIDEntry),
						widget.NewFormItem("Confirmed save path", saveEntry),
					),
					widget.NewLabel("Detected candidates"),
					candidates,
					container.NewHBox(saveMapBtn, backupBtn),
				),
			)
			gameList.Add(card)
		}
	}

	scroll := container.NewVScroll(gameList)
	scroll.SetMinSize(fyne.NewSize(1000, 420))

	s.mainBody = container.NewBorder(
		container.NewVBox(
			widget.NewLabel("emusave.jey"),
			top,
		),
		s.statusText,
		nil,
		nil,
		container.NewVBox(scroll),
	)

	if len(s.games) == 0 {
		s.setStatus("Ready. Click Rescan games to load ROMs and candidate save paths.")
	}
}

func (s *state) renderSettings() {
	backBtn := widget.NewButton("Back to main", func() {
		s.showMain()
	})

	saveBtn := widget.NewButton("Save settings", func() {
		if err := s.saveConfigFromWidgets(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}
		s.setStatus("Settings saved.")
	})

	autoBtn := widget.NewButton("Use default Ryujinx config", func() {
		s.ryuCfgEntry.SetText(util.DefaultRyujinxConfigPath())
	})

	pickCfgBtn := widget.NewButton("Choose Ryujinx config.json", func() {
		s.chooseRyujinxConfigFile()
	})

	pickLocalBtn := widget.NewButton("Choose local folder", func() {
		s.chooseLocalFolder()
	})

	browseSMBBtn := widget.NewButton("Browse SMB path", func() {
		s.browseSMBPath()
	})

	testBtn := widget.NewButton("Test selected target", func() {
		if err := s.saveConfigFromWidgets(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}
		if err := s.validateCurrentMode(); err != nil {
			dialog.ShowError(err, s.mainWindow)
			return
		}

		switch s.cfg.SyncMode {
		case appconfig.SyncModeSMB:
			client, err := smbclient.Connect(s.cfg.SMB, util.DecodeBase64(s.cfg.SMB.PasswordBase64))
			if err != nil {
				dialog.ShowError(err, s.mainWindow)
				return
			}
			defer client.Close()

			if err := client.TestWrite(s.cfg.SMB.BaseDir); err != nil {
				dialog.ShowError(err, s.mainWindow)
				return
			}
			dialog.ShowInformation("Success", "SMB login and test write worked.", s.mainWindow)

		case appconfig.SyncModeLocal:
			p := util.ExpandUserPath(s.cfg.Local.BasePath)
			if p == "" {
				dialog.ShowError(fmt.Errorf("local path is empty"), s.mainWindow)
				return
			}
			if err := os.MkdirAll(p, 0o755); err != nil {
				dialog.ShowError(fmt.Errorf("cannot create local base path: %w", err), s.mainWindow)
				return
			}
			dialog.ShowInformation("Success", "Local backup path is usable.", s.mainWindow)

		case appconfig.SyncModeCommand:
			dialog.ShowInformation("Command mode", "Command is set.", s.mainWindow)
		}
	})

	form := container.NewVBox(
		widget.NewLabel("Ryujinx"),
		widget.NewForm(
			widget.NewFormItem("Config.json", s.ryuCfgEntry),
			widget.NewFormItem("Data root", s.ryuDataEntry),
		),
		container.NewHBox(autoBtn, pickCfgBtn),
		widget.NewSeparator(),

		widget.NewLabel("Sync mode"),
		s.syncModeRadio,
		widget.NewSeparator(),

		widget.NewLabel("SMB"),
		widget.NewForm(
			widget.NewFormItem("IP / Host", s.smbHostEntry),
			widget.NewFormItem("Username", s.smbUserEntry),
			widget.NewFormItem("Password", s.smbPassEntry),
			widget.NewFormItem("Domain", s.smbDomainEntry),
			widget.NewFormItem("Share", s.smbShareEntry),
			widget.NewFormItem("Path in share", s.smbBaseEntry),
		),
		container.NewHBox(browseSMBBtn),
		widget.NewSeparator(),

		widget.NewLabel("Local"),
		widget.NewForm(
			widget.NewFormItem("Base path", s.localBaseEntry),
		),
		container.NewHBox(pickLocalBtn),
		widget.NewSeparator(),

		widget.NewLabel("Custom command"),
		widget.NewForm(
			widget.NewFormItem("Command", s.commandEntry),
		),
		widget.NewLabel("Placeholders: {save_path} {title_name} {title_id} {rom_path} {timestamp}"),
		container.NewHBox(backBtn, testBtn, saveBtn),
	)

	scroll := container.NewVScroll(form)
	scroll.SetMinSize(fyne.NewSize(1000, 520))

	s.mainBody = container.NewBorder(
		container.NewVBox(
			widget.NewLabel("Settings"),
		),
		s.statusText,
		nil,
		nil,
		container.NewVBox(scroll),
	)
}
