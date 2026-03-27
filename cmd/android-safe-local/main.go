package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/cespare/xxhash/v2"

	"AndroidSafeLocal/internal/adb"
	"AndroidSafeLocal/internal/backup"
	"AndroidSafeLocal/internal/dedup"
	device_pkg "AndroidSafeLocal/internal/device"
	"AndroidSafeLocal/internal/gallery"
	"AndroidSafeLocal/internal/i18n"
	"AndroidSafeLocal/internal/manifest"
	"AndroidSafeLocal/internal/sorter"
)

func main() {
	a := app.NewWithID("com.androidsafelocal.app")
	a.Settings().SetTheme(&midnightTheme{})

	savedLang := a.Preferences().StringWithFallback("language", "en")
	if savedLang == "es" {
		i18n.SetLanguage(i18n.Spanish)
	} else {
		i18n.SetLanguage(i18n.English)
	}
	t := i18n.T()

	w := a.NewWindow(t.AppTitle)
	w.Resize(fyne.NewSize(960, 680))

	// STATUS BAR (U2)
	statusBarDevice := widget.NewLabel("●  " + t.CheckingConn)
	statusBarSpeed := widget.NewLabel("")
	statusBarETA := widget.NewLabel("")
	statusBarFree := widget.NewLabel(t.StatusFreeErr)
	statusBarWorkers := widget.NewLabel("")
	statusBar := container.NewHBox(
		statusBarDevice, widget.NewSeparator(),
		statusBarWorkers, widget.NewSeparator(),
		statusBarSpeed, widget.NewSeparator(),
		statusBarETA, widget.NewSeparator(),
		statusBarFree,
	)

	// SHARED STATE
	var stateMu sync.RWMutex
	var client *adb.Client
	var files []device_pkg.File

	// LOGS
	logArea := widget.NewMultiLineEntry()
	logArea.SetMinRowsVisible(6)
	logArea.Disable()
	var logMu sync.Mutex
	logPrint := func(msg string) {
		logMu.Lock()
		defer logMu.Unlock()
		ts := time.Now().Format("15:04:05")
		logArea.SetText(logArea.Text + fmt.Sprintf("[%s] %s\n", ts, msg))
		logArea.Refresh()
	}

	// PROGRESS
	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	// BUTTONS (declared early for setActionsEnabled)
	var scanBtn, backupBtn, galleryBtn, restoreBtn, previewBtn *widget.Button
	setActionsEnabled := func(enabled bool) {
		for _, btn := range []*widget.Button{scanBtn, backupBtn, galleryBtn, restoreBtn, previewBtn} {
			if btn == nil {
				continue
			}
			if enabled {
				btn.Enable()
			} else {
				btn.Disable()
			}
		}
	}
	backgroundOp := func(action func()) {
		setActionsEnabled(false)
		go func() {
			defer setActionsEnabled(true)
			action()
		}()
	}

	// SOURCE / DEST
	sourceEntry := widget.NewEntry()
	sourceEntry.SetText(a.Preferences().StringWithFallback("source", "/sdcard/DCIM"))
	sourceSelect := widget.NewSelect([]string{
		"/sdcard",
		"/sdcard/DCIM",
		"/sdcard/Download",
		"/sdcard/Downloads",
		"/sdcard/Pictures",
		"/sdcard/Music",
		"/sdcard/Movies",
		"/sdcard/Podcasts",
		"/sdcard/WhatsApp/Media",
		"/sdcard/Telegram",
		"/sdcard/Signal",
		"/sdcard/Instagram",
		"/storage/emulated/0",
		"/storage/emulated/0/DCIM",
		"/storage/emulated/0/Pictures",
		"/storage/emulated/0/Downloads",
		"/storage/sdcard1",
		"/storage/sdcard1/DCIM",
	}, func(s string) { sourceEntry.SetText(s) })
	sourceSelect.PlaceHolder = t.QuickSelect

	destEntry := widget.NewEntry()
	destEntry.SetText(a.Preferences().StringWithFallback("dest", `C:\Backup\Android`))

	// F1: WORKERS SLIDER
	workersValue := a.Preferences().IntWithFallback("workers", 5)
	workersLabel := widget.NewLabel(fmt.Sprintf(t.StatusWorkers, workersValue))
	workersSlider := widget.NewSlider(1, 10)
	workersSlider.Step = 1
	workersSlider.SetValue(float64(workersValue))
	workersSlider.OnChanged = func(v float64) {
		workersValue = int(v)
		workersLabel.SetText(fmt.Sprintf(t.StatusWorkers, workersValue))
		statusBarWorkers.SetText(fmt.Sprintf(t.StatusWorkers, workersValue))
	}
	statusBarWorkers.SetText(fmt.Sprintf(t.StatusWorkers, workersValue))

	// DOCS / OPTIONS
	includeDocsCheck := widget.NewCheck(t.IncludeDocs, nil)
	includeDocsCheck.SetChecked(a.Preferences().BoolWithFallback("includeDocs", false))

	// F5: OPEN IN BROWSER
	openInBrowserCheck := widget.NewCheck(t.OpenInBrowser, nil)
	openInBrowserCheck.SetChecked(a.Preferences().BoolWithFallback("openInBrowser", false))

	// V1: VERIFY TRANSFERRED FILES
	verifyCheck := widget.NewCheck(t.VerifyFiles, nil)
	verifyCheck.SetChecked(a.Preferences().BoolWithFallback("verifyFiles", false))

	// E2: EXCLUDE PATTERNS — checkboxes + custom entry
	presetLabels := make([]string, len(excludePresets))
	for i, p := range excludePresets {
		presetLabels[i] = p.label
	}
	excludePresetChecks := widget.NewCheckGroup(presetLabels, nil)
	excludePresetChecks.Horizontal = false
	savedPresetsSel := a.Preferences().StringWithFallback("excludePresetsSel", "")
	if savedPresetsSel != "" {
		excludePresetChecks.Selected = strings.Split(savedPresetsSel, "|")
	}
	excludeCustomEntry := widget.NewEntry()
	excludeCustomEntry.SetPlaceHolder(t.ExcludePlaceholder)
	excludeCustomEntry.SetText(a.Preferences().StringWithFallback("excludeCustom", ""))
	getExcludePatterns := func() []string {
		selSet := make(map[string]bool, len(excludePresetChecks.Selected))
		for _, s := range excludePresetChecks.Selected {
			selSet[s] = true
		}
		var patterns []string
		for _, p := range excludePresets {
			if selSet[p.label] {
				patterns = append(patterns, p.pattern)
			}
		}
		patterns = append(patterns, parseExcludePatterns(excludeCustomEntry.Text)...)
		return patterns
	}

	// LANGUAGE
	currentLangSelection := "English"
	if i18n.GetLanguage() == i18n.Spanish {
		currentLangSelection = "Español"
	}
	langSelect := widget.NewSelect([]string{"English", "Español"}, func(selected string) {
		if selected == currentLangSelection {
			return
		}
		currentLangSelection = selected
		var newLang i18n.Language
		if selected == "Español" {
			newLang = i18n.Spanish
		} else {
			newLang = i18n.English
		}
		a.Preferences().SetString("language", string(newLang))
		dialog.ShowInformation(t.Language,
			"Please restart the application to apply the new language.\n"+
				"Por favor, reinicia la aplicación para aplicar el nuevo idioma.", w)
	})
	langSelect.SetSelected(currentLangSelection)

	// F2: FREE DISK SPACE
	updateFreeSpace := func() {
		free, err := diskFreeGB(destEntry.Text)
		if err != nil {
			statusBarFree.SetText(t.StatusFreeErr)
		} else {
			statusBarFree.SetText(fmt.Sprintf(t.StatusFreeGB, free))
		}
	}
	destEntry.OnChanged = func(_ string) { go updateFreeSpace() }
	go updateFreeSpace()

	// F4: PROFILES
	profileNames := loadProfileNames(a)
	profileSelect := widget.NewSelect(profileNames, nil)
	if len(profileNames) > 0 {
		profileSelect.SetSelected(profileNames[0])
	}
	profileSelect.PlaceHolder = t.ProfileLabel
	saveProfileBtn := widget.NewButtonWithIcon(t.ProfileSave, theme.DocumentSaveIcon(), func() {
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder(t.ProfileNew)
		formDialog := dialog.NewForm(
			t.ProfileSave, t.ProfileSave, t.Cancel,
			[]*widget.FormItem{widget.NewFormItem(t.ProfileNew, nameEntry)},
			func(ok bool) {
				if !ok || nameEntry.Text == "" {
					return
				}
				saveProfile(a, nameEntry.Text, sourceEntry.Text, destEntry.Text, workersValue,
						includeDocsCheck.Checked,
						strings.Join(excludePresetChecks.Selected, "|"),
						excludeCustomEntry.Text)
				logPrint(t.ProfileSaved)
				names := loadProfileNames(a)
				profileSelect.Options = names
				profileSelect.SetSelected(nameEntry.Text)
				profileSelect.Refresh()
			}, w)
		formDialog.Show()
	})
	// P2: PROFILE EXPORT / IMPORT
	exportProfilesBtn := widget.NewButtonWithIcon(t.ProfileExport, theme.UploadIcon(), func() {
		saveDlg := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
			if err != nil || uc == nil {
				return
			}
			defer uc.Close()
			names := loadProfileNames(a)
			var exported []exportedProfile
			for _, name := range names {
				src, dst, workers, docs, ep, ec := loadProfile(a, name)
				exported = append(exported, exportedProfile{Name: name, Src: src, Dst: dst, Workers: workers, Docs: docs, ExclPresets: ep, ExclCustom: ec})
			}
			b, mErr := json.MarshalIndent(exported, "", "  ")
			if mErr != nil {
				logPrint(fmt.Sprintf(t.ProfileImportErr, mErr.Error()))
				return
			}
			if _, wErr := uc.Write(b); wErr != nil {
				logPrint(fmt.Sprintf(t.ProfileImportErr, wErr.Error()))
				return
			}
			logPrint(fmt.Sprintf(t.ProfileExportDone, uc.URI().Name()))
		}, w)
		saveDlg.SetFileName("profiles.json")
		saveDlg.Show()
	})
	importProfilesBtn := widget.NewButtonWithIcon(t.ProfileImport, theme.DownloadIcon(), func() {
		dialog.ShowFileOpen(func(uc fyne.URIReadCloser, err error) {
			if err != nil || uc == nil {
				return
			}
			defer uc.Close()
			data, rErr := io.ReadAll(uc)
			if rErr != nil {
				logPrint(fmt.Sprintf(t.ProfileImportErr, rErr.Error()))
				return
			}
			var imported []exportedProfile
			if uErr := json.Unmarshal(data, &imported); uErr != nil {
				logPrint(fmt.Sprintf(t.ProfileImportErr, uErr.Error()))
				return
			}
			for _, p := range imported {
				saveProfile(a, p.Name, p.Src, p.Dst, p.Workers, p.Docs, p.ExclPresets, p.ExclCustom)
			}
			allNames := loadProfileNames(a)
			profileSelect.Options = allNames
			profileSelect.Refresh()
			logPrint(fmt.Sprintf(t.ProfileImportDone, len(imported)))
		}, w)
	})
	profileSelect.OnChanged = func(name string) {
		if name == "" {
			return
		}
		src, dst, workers, docs, exclPresets, exclCustom := loadProfile(a, name)
		sourceEntry.SetText(src)
		destEntry.SetText(dst)
		workersSlider.SetValue(float64(workers))
		includeDocsCheck.SetChecked(docs)
		if exclPresets != "" {
			excludePresetChecks.Selected = strings.Split(exclPresets, "|")
			excludePresetChecks.Refresh()
		} else {
			excludePresetChecks.Selected = nil
			excludePresetChecks.Refresh()
		}
		excludeCustomEntry.SetText(exclCustom)
		go updateFreeSpace()
	}

	// E1: PRE-BACKUP STATS LABELS
	statsNewLabel := widget.NewLabel("")
	statsSkipLabel := widget.NewLabel("")
	statsMBLabel := widget.NewLabel("")
	statsBox := container.NewHBox(statsNewLabel, statsSkipLabel, statsMBLabel)
	updateStats := func(localFiles []device_pkg.File, includeDocs bool) {
		registry := dedup.NewRegistry()
		_ = registry.Load(destEntry.Text)
		newCount, skipCount := 0, 0
		var pendingBytes int64
		for _, f := range localFiles {
			if f.IsDir || !shouldBackupFile(f.Path, includeDocs) {
				continue
			}
			devFile := device_pkg.File{Path: f.Path, Size: f.Size}
			if registry.Exists(devFile) {
				skipCount++
			} else {
				newCount++
				pendingBytes += f.Size
			}
		}
		statsNewLabel.SetText(fmt.Sprintf(t.StatsNew, newCount))
		statsSkipLabel.SetText(fmt.Sprintf(t.StatsSkipped, skipCount))
		statsMBLabel.SetText(fmt.Sprintf(t.StatsMB, float64(pendingBytes)/1e6))
	}

	// N2: POST-OP ACTION BAR — shown after backup/gallery completes
	var postOpDestRoot string
	postOpOpenFolderBtn := widget.NewButtonWithIcon(t.OpenFolder, theme.FolderOpenIcon(), func() {
		_ = exec.Command("explorer", filepath.FromSlash(postOpDestRoot)).Start()
	})
	postOpOpenGalleryBtn := widget.NewButtonWithIcon(t.OpenGallery, theme.MediaPhotoIcon(), func() {
		_ = exec.Command("cmd", "/c", "start", "", filepath.Join(postOpDestRoot, "index.html")).Start()
	})
	postOpRow := container.NewHBox(postOpOpenFolderBtn, postOpOpenGalleryBtn)
	postOpRow.Hide()

	// SCAN
	scanBtn = widget.NewButtonWithIcon(t.ScanFiles, theme.SearchIcon(), func() {
		stateMu.RLock()
		c := client
		stateMu.RUnlock()
		if c == nil {
			dialog.ShowError(errors.New("ADB not initialized"), w)
			return
		}
		logPrint(t.Scanning + " " + sourceEntry.Text + "...")
		progressBar.Show()
		backgroundOp(func() {
			walker := device_pkg.NewWalker(c)
			result, err := walker.Walk(sourceEntry.Text)
			if err != nil {
				logPrint(t.ScanFailed + ": " + err.Error())
				progressBar.Hide()
				return
			}
			stateMu.Lock()
			files = result
			stateMu.Unlock()
			logPrint(fmt.Sprintf(t.FoundFiles, len(result)))
			progressBar.Hide()
			go updateFreeSpace()
			go updateStats(result, includeDocsCheck.Checked)
		})
	})

	// PREVIEW / DRY-RUN (D1)
	previewBtn = widget.NewButtonWithIcon(t.DryRun, theme.VisibilityIcon(), func() {
		stateMu.RLock()
		localFiles := append([]device_pkg.File(nil), files...)
		stateMu.RUnlock()
		if len(localFiles) == 0 {
			dialog.ShowInformation(t.InfoTitle, t.ScanFirst, w)
			return
		}
		includeDocs := includeDocsCheck.Checked
		excludePatterns := getExcludePatterns()
		destRoot := destEntry.Text
		dryReg := dedup.NewRegistry()
		if existingManifest, err := manifest.Load(destRoot); err == nil {
			dryReg.LoadHashes(existingManifest.HashSet())
		}
		_ = dryReg.Load(destRoot)
		var toTransfer []string
		var skipCount int
		var totalBytes int64
		for _, f := range localFiles {
			if f.IsDir || !shouldBackupFile(f.Path, includeDocs) || isExcluded(f.Path, excludePatterns) {
				continue
			}
			devFile := device_pkg.File{Path: f.Path, Size: f.Size}
			if dryReg.Exists(devFile) {
				skipCount++
			} else {
				toTransfer = append(toTransfer, fmt.Sprintf("%-40s  %.1f MB", filepath.Base(f.Path), float64(f.Size)/1e6))
				totalBytes += f.Size
			}
		}
		var content string
		if len(toTransfer) == 0 {
			content = t.DryRunEmpty
		} else {
			shown := toTransfer
			more := ""
			if len(shown) > 200 {
				more = fmt.Sprintf("\n\n... +%d more", len(shown)-200)
				shown = shown[:200]
			}
			content = fmt.Sprintf(t.DryRunHeader, len(toTransfer), float64(totalBytes)/1e6, skipCount) +
				"\n\n" + strings.Join(shown, "\n") + more
		}
		previewArea := widget.NewMultiLineEntry()
		previewArea.SetText(content)
		previewArea.Disable()
		previewArea.SetMinRowsVisible(15)
		d := dialog.NewCustom(t.DryRunTitle, t.Cancel, container.NewVScroll(previewArea), w)
		d.Resize(fyne.NewSize(540, 440))
		d.Show()
	})

	// BACKUP
	backupBtn = widget.NewButtonWithIcon(t.StartBackup, theme.DownloadIcon(), func() {
		stateMu.RLock()
		localFiles := append([]device_pkg.File(nil), files...)
		stateMu.RUnlock()
		if len(localFiles) == 0 {
			dialog.ShowInformation(t.InfoTitle, t.ScanFirst, w)
			return
		}
		includeDocs := includeDocsCheck.Checked
		excludePatterns := getExcludePatterns()
		destRoot := destEntry.Text
		workers := workersValue
		eligibleCount := 0
		for _, f := range localFiles {
			if !f.IsDir && shouldBackupFile(f.Path, includeDocs) && !isExcluded(f.Path, excludePatterns) {
				eligibleCount++
			}
		}
		stateMu.RLock()
		c := client
		stateMu.RUnlock()

		startBackup := func(resume bool) {
			verifyFiles := verifyCheck.Checked
			var totalBackupBytes int64
			for _, f := range localFiles {
				if !f.IsDir && shouldBackupFile(f.Path, includeDocs) && !isExcluded(f.Path, excludePatterns) {
					totalBackupBytes += f.Size
				}
			}
			logPrint(t.StartingBackup)
			progressBar.SetValue(0)
			progressBar.Max = float64(eligibleCount)
			progressBar.Show()
			backgroundOp(func() {
				startTime := time.Now()
				var completedPaths map[string]bool
				if resume {
					if state, err := backup.LoadState(destRoot); err == nil {
						completedPaths = state.CompletedPaths
						logPrint(t.Resuming)
					}
				} else {
					_ = backup.ClearState(destRoot)
				}
				if completedPaths == nil {
					completedPaths = make(map[string]bool)
				}
				runState := backup.NewBackupState()

				registry := dedup.NewRegistry()
				logPrint(t.LoadingIndex)
				if existingManifest, err := manifest.Load(destRoot); err == nil {
					registry.LoadHashes(existingManifest.HashSet())
				}
				if err := registry.Load(destRoot); err != nil {
					logPrint(t.RegistryWarning + ": " + err.Error())
				}

				agent := &backup.TransferAgent{Client: c}
				pool := backup.NewPool(workers, agent, registry)
				pool.Start()
				fileSorter := sorter.NewSorter()
				backupManifest := manifest.New()

				// U3: speed tracking + E3: ETA
				var bytesTransferred atomic.Int64
				speedTicker := time.NewTicker(time.Second)
				go func() {
					var lastBytes int64
					for range speedTicker.C {
						current := bytesTransferred.Load()
						mbps := float64(current-lastBytes) / 1e6
						lastBytes = current
						if mbps > 0 {
							statusBarSpeed.SetText(fmt.Sprintf(t.StatusSpeed, mbps))
						}
						elapsed := time.Since(startTime).Seconds()
						if elapsed > 2 && current > 0 && totalBackupBytes > current {
							bps := float64(current) / elapsed
							etaSecs := float64(totalBackupBytes-current) / bps
							statusBarETA.SetText(fmt.Sprintf(t.StatusETA, formatETA(etaSecs)))
						} else if current > 0 {
							statusBarETA.SetText(t.StatusETACalc)
						}
					}
				}()

				failures, success := 0, 0
				go func() {
					for _, f := range localFiles {
						if f.IsDir || !shouldBackupFile(f.Path, includeDocs) || isExcluded(f.Path, excludePatterns) || completedPaths[f.Path] {
							continue
						}
						var relDest string
						if includeDocs && sorter.IsDocument(f.Path) {
							relDest = fileSorter.GetDocumentDestination(f, sourceEntry.Text)
						} else {
							relDest = fileSorter.GetDestination(f)
						}
						pool.AddJob(backup.Job{
							SourcePath: f.Path,
							DestPath:   filepath.Join(destRoot, relDest),
							Size:       f.Size,
							Timestamp:  f.Timestamp,
						})
					}
					pool.Close()
				}()

				processed := 0
				for res := range pool.Results() {
					if res.Error != nil {
						logPrint(fmt.Sprintf("%s: %s (%v)", t.Fail, filepath.Base(res.Job.SourcePath), res.Error))
						failures++
					} else if res.Skipped {
						logPrint(fmt.Sprintf("%s: %s", t.Skip, filepath.Base(res.Job.SourcePath)))
						success++
					} else {
						actualDest := res.Job.DestPath
						if !sorter.IsDocument(res.Job.SourcePath) {
							devFile := device_pkg.File{Path: res.Job.SourcePath, Timestamp: res.Job.Timestamp}
							exifRel := fileSorter.GetDestinationWithEXIF(devFile, res.Job.DestPath)
							exifFull := filepath.Join(destRoot, exifRel)
							if exifFull != res.Job.DestPath {
								if mkErr := os.MkdirAll(filepath.Dir(exifFull), 0755); mkErr == nil {
									if renErr := os.Rename(res.Job.DestPath, exifFull); renErr == nil {
										actualDest = exifFull
									}
								}
							}
						}
						hash, _ := xxhashFile(actualDest)
						if hash != "" {
							registry.AddByHash(hash)
						}
						// V1: optional integrity verification
						if verifyFiles && hash != "" {
							if hash2, _ := xxhashFile(actualDest); hash2 != hash {
								logPrint(fmt.Sprintf(t.VerifyFail, filepath.Base(actualDest)))
							}
						}
						bytesTransferred.Add(res.Job.Size)
						runState.CompletedPaths[res.Job.SourcePath] = true
						if len(runState.CompletedPaths)%10 == 0 {
							_ = runState.Save(destRoot)
						}
						relPath, _ := filepath.Rel(destRoot, actualDest)
						backupManifest.Add(res.Job.SourcePath, relPath, res.Job.Size, res.Job.Timestamp, hash)
						success++
					}
					processed++
					progressBar.SetValue(float64(processed))
				}
				speedTicker.Stop()
				statusBarSpeed.SetText("")
				statusBarETA.SetText("")
				logPrint(fmt.Sprintf(t.Finished, success, failures))
				if err := backupManifest.Save(destRoot); err != nil {
					logPrint(t.ManifestSaveFail + ": " + err.Error())
				} else {
					logPrint(t.ManifestSaved)
				}
				_ = backup.ClearState(destRoot)
				progressBar.Hide()
				postOpDestRoot = destRoot
				postOpRow.Show()
				go updateFreeSpace()
				go updateStats(localFiles, includeDocs)
				// F3: notification
				a.SendNotification(&fyne.Notification{
					Title:   t.NotifyTitle,
					Content: fmt.Sprintf(t.NotifyBackup, success),
				})
				a.Preferences().SetString("source", sourceEntry.Text)
				a.Preferences().SetString("dest", destRoot)
				a.Preferences().SetInt("workers", workers)
				a.Preferences().SetBool("includeDocs", includeDocs)
				a.Preferences().SetString("excludePresetsSel", strings.Join(excludePresetChecks.Selected, "|"))
				a.Preferences().SetString("excludeCustom", excludeCustomEntry.Text)
				a.Preferences().SetBool("verifyFiles", verifyFiles)
			})
		}

		if backup.StateExists(destRoot) {
			dialog.NewCustomConfirm(
				t.ResumeTitle, t.ResumeYes, t.ResumeNo,
				widget.NewLabel(t.ResumeMsg),
				func(resume bool) { startBackup(resume) }, w,
			).Show()
		} else {
			startBackup(false)
		}
	})

	// GALLERY
	galleryBtn = widget.NewButtonWithIcon(t.GenerateGallery, theme.MediaPhotoIcon(), func() {
		dest := destEntry.Text
		openWhenDone := openInBrowserCheck.Checked
		logPrint(t.GeneratingGallery)
		progressBar.SetValue(0)
		progressBar.Max = 1
		progressBar.Show()
		backgroundOp(func() {
			gen := gallery.NewGenerator()
			count, err := gen.Generate(dest, func(current, total int) {
				progressBar.Max = float64(total)
				progressBar.SetValue(float64(current))
			})
			if err != nil {
				if count > 0 {
					logPrint(fmt.Sprintf(t.GalleryIncomplete, count, err.Error()))
				} else {
					logPrint(t.GalleryError + ": " + err.Error())
				}
			} else {
				if count == 0 {
					logPrint(t.NoMediaFiles)
				} else {
					logPrint(fmt.Sprintf(t.GalleryCreated, count))
					if openWhenDone {
						// F5: open in browser
						_ = exec.Command("cmd", "/c", "start", "", filepath.Join(dest, "index.html")).Start()
					}
					// N2: post-op action bar
					postOpDestRoot = dest
					postOpRow.Show()
					// F3: notification
					a.SendNotification(&fyne.Notification{
						Title:   t.NotifyTitle,
						Content: fmt.Sprintf(t.NotifyGallery, count),
					})
				}
			}
			progressBar.Hide()
		})
	})

	// RESTORE
	restoreBtn = widget.NewButtonWithIcon(t.Restore, theme.UploadIcon(), func() {
		stateMu.RLock()
		c := client
		stateMu.RUnlock()
		if c == nil {
			dialog.ShowError(errors.New("ADB not initialized"), w)
			return
		}
		localPath := destEntry.Text
		backupManifest, err := manifest.Load(localPath)
		if err != nil {
			remotePath := "/sdcard/Restored"
			dialog.NewCustomConfirm(
				t.ConfirmRestore, t.RestoreNow, t.Cancel,
				widget.NewLabel(fmt.Sprintf(t.NoManifestMsg, remotePath)),
				func(confirmed bool) {
					if !confirmed {
						logPrint(t.RestoreCancelled)
						return
					}
					logPrint(t.RestoringTo + " " + remotePath + "...")
					progressBar.Show()
					backgroundOp(func() {
						if err := c.Push(localPath, remotePath); err != nil {
							logPrint(t.RestoreFailed + ": " + err.Error())
						} else {
							logPrint(fmt.Sprintf(t.RestoreComplete, 1, 0) + " " + remotePath)
						}
						progressBar.Hide()
					})
				}, w).Show()
			return
		}
		dialog.NewCustomConfirm(
			t.ConfirmRestore, t.RestoreToOriginal, t.Cancel,
			widget.NewLabel(fmt.Sprintf(t.ManifestFoundMsg, len(backupManifest.Entries))),
			func(confirmed bool) {
				if !confirmed {
					logPrint(t.RestoreCancelled)
					return
				}
				logPrint(t.RestoringToOriginal)
				progressBar.SetValue(0)
				progressBar.Max = float64(len(backupManifest.Entries))
				progressBar.Show()
				backgroundOp(func() {
					restorePool := backup.NewRestorePool(15, c)
					restorePool.Start()
					total := len(backupManifest.Entries)
					go func() {
						for i, entry := range backupManifest.Entries {
							restorePool.AddJob(backup.RestoreJob{
								LocalPath:    filepath.Join(localPath, entry.LocalPath),
								OriginalPath: entry.OriginalPath,
								Index:        i + 1,
								Total:        total,
							})
						}
						restorePool.Close()
					}()
					success, failures, lastLogged := 0, 0, 0
					logInterval := max(1, total/20)
					for res := range restorePool.Results() {
						if res.Error != nil {
							logPrint(fmt.Sprintf("✗ %s: %s - %s", t.Fail, filepath.Base(res.Job.LocalPath), res.Error.Error()))
							failures++
						} else {
							success++
						}
						processed := success + failures
						progressBar.SetValue(float64(processed))
						if processed-lastLogged >= logInterval || processed == total {
							logPrint(fmt.Sprintf(t.Progress, processed, total))
							lastLogged = processed
						}
					}
					logPrint(fmt.Sprintf(t.RestoreComplete, success, failures))
					progressBar.Hide()
				})
			}, w).Show()
	})

	// CONFIG CARD
	configCard := widget.NewCard(t.Configuration, "", container.NewVBox(
		widget.NewLabelWithStyle(t.SourcePath, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, sourceSelect, sourceEntry),
		widget.NewLabelWithStyle(t.DestPath, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		destEntry,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(t.FileTypes, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		includeDocsCheck,
		verifyCheck,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(t.ExcludeLabel, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle(t.ExcludePresets, fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		excludePresetChecks,
		widget.NewLabelWithStyle(t.ExcludeCustom, fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		excludeCustomEntry,
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabelWithStyle(t.WorkersLabel, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			workersLabel,
		),
		workersSlider,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(t.Language, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		langSelect,
		widget.NewSeparator(),
		container.NewHBox(
			widget.NewLabelWithStyle(t.ProfileLabel, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			profileSelect,
			saveProfileBtn,
			exportProfilesBtn,
			importProfilesBtn,
		),
	))

	logItem := widget.NewAccordionItem(t.ActivityLog, logArea)
	logItem.Open = true
	logAccordion := widget.NewAccordion(logItem)

	// U1: TABS
	backupTab := container.NewVBox(
		configCard,
		widget.NewCard(t.Actions, "", container.NewVBox(
			container.NewGridWithColumns(3, scanBtn, previewBtn, backupBtn),
			statsBox,
			postOpRow,
		)),
		progressBar,
		widget.NewSeparator(),
		logAccordion,
	)
	galleryTab := container.NewVBox(
		widget.NewCard(t.GenerateGallery, "", container.NewVBox(
			widget.NewLabelWithStyle(t.DestPath, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			destEntry,
			openInBrowserCheck,
			galleryBtn,
		)),
		progressBar,
		widget.NewSeparator(),
		logAccordion,
	)
	restoreTab := container.NewVBox(
		widget.NewCard(t.Restore, "", container.NewVBox(
			widget.NewLabelWithStyle(t.DestPath, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			destEntry,
			restoreBtn,
		)),
		progressBar,
		widget.NewSeparator(),
		logAccordion,
	)
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon(t.TabBackup, theme.DownloadIcon(), backupTab),
		container.NewTabItemWithIcon(t.TabGallery, theme.MediaPhotoIcon(), galleryTab),
		container.NewTabItemWithIcon(t.TabRestore, theme.UploadIcon(), restoreTab),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	// SIDEBAR
	statusLabel := widget.NewLabel(t.CheckingConn)
	statusLabel.Wrapping = fyne.TextWrapWord
	deviceIcon := widget.NewIcon(theme.ComputerIcon())
	sidebarCard := widget.NewCard(t.DeviceStatus, "", container.NewVBox(
		container.NewHBox(deviceIcon, widget.NewLabel(t.AndroidDevice)),
		statusLabel,
	))
	sidebar := container.NewVBox(sidebarCard, widget.NewSeparator())

	split := container.NewHSplit(sidebar, tabs)
	split.SetOffset(0.25)
	root := container.NewBorder(nil, statusBar, nil, nil, split)
	w.SetContent(root)

	// INIT
	go func() {
		newClient, err := adb.NewClient()
		if err != nil {
			statusLabel.SetText(t.ADBNotFound)
			statusBarDevice.SetText("✕  " + t.ADBNotFound)
			logPrint(t.ADBError + ": " + err.Error())
			return
		}
		stateMu.Lock()
		client = newClient
		stateMu.Unlock()
		devices, err := newClient.Devices()
		if err != nil {
			statusLabel.SetText(t.ADBError + ": " + err.Error())
			statusBarDevice.SetText("✕  " + t.ADBError)
			return
		}
		if len(devices) > 0 {
			statusLabel.SetText(fmt.Sprintf("%s:\n%s\n%s", t.Connected, devices[0].Model, devices[0].Serial))
			statusLabel.TextStyle = fyne.TextStyle{Bold: true}
			statusBarDevice.SetText("●  " + devices[0].Model + " (" + devices[0].Serial + ")")
			logPrint(t.DeviceConnected + ": " + devices[0].Serial)
		} else {
			statusLabel.SetText(t.NoDevice)
			statusBarDevice.SetText("○  " + t.NoDevice)
			logPrint(t.WaitingDevice)
		}
	}()

	w.SetOnClosed(func() {
		stateMu.RLock()
		closingClient := client
		stateMu.RUnlock()
		if closingClient != nil {
			if err := closingClient.KillServer(); err != nil {
				logPrint(t.ADBError + ": " + err.Error())
			}
		}
	})

	w.ShowAndRun()
}

// xxhashFile computes the xxHash (hex string) of a local file.
func xxhashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := xxhash.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum64()), nil
}

// parseExcludePatterns splits comma-separated patterns into a lowercase slice.
func parseExcludePatterns(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(strings.ToLower(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// isExcluded returns true when filePath matches any pattern.
// Supports glob patterns against the basename (e.g. *.tmp, thumb*)
// and plain substring matches against the full path (e.g. /android/data/).
func isExcluded(filePath string, patterns []string) bool {
	lowerPath := strings.ToLower(filepath.ToSlash(filePath))
	base := strings.ToLower(filepath.Base(filePath))
	for _, p := range patterns {
		if strings.ContainsAny(p, "*?[") {
			if ok, _ := filepath.Match(p, base); ok {
				return true
			}
		}
		if strings.Contains(lowerPath, p) {
			return true
		}
	}
	return false
}

// formatETA converts seconds to a human-readable duration string.
func formatETA(secs float64) string {
	if secs < 0 {
		secs = 0
	}
	d := time.Duration(int64(secs)) * time.Second
	if d >= time.Hour {
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// diskFreeGB returns the free disk space in GB for the volume containing path.
func diskFreeGB(path string) (float64, error) {
	if path == "" {
		return 0, errors.New("empty path")
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		path = filepath.Dir(path)
	}
	return diskFreeGBOS(path)
}

// F4 profile helpers
func profileKey(name, field string) string { return "profile." + name + "." + field }

func saveProfile(a fyne.App, name, src, dst string, workers int, docs bool, exclPresets, exclCustom string) {
	a.Preferences().SetString(profileKey(name, "src"), src)
	a.Preferences().SetString(profileKey(name, "dst"), dst)
	a.Preferences().SetInt(profileKey(name, "workers"), workers)
	a.Preferences().SetBool(profileKey(name, "docs"), docs)
	a.Preferences().SetString(profileKey(name, "excl"), exclPresets)
	a.Preferences().SetString(profileKey(name, "excl_custom"), exclCustom)
	names := loadProfileNames(a)
	for _, n := range names {
		if n == name {
			return
		}
	}
	a.Preferences().SetString("profile.names", strings.Join(append(names, name), "|"))
}

func loadProfileNames(a fyne.App) []string {
	raw := a.Preferences().StringWithFallback("profile.names", "")
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "|")
}

func loadProfile(a fyne.App, name string) (src, dst string, workers int, docs bool, exclPresets, exclCustom string) {
	src = a.Preferences().StringWithFallback(profileKey(name, "src"), "/sdcard/DCIM")
	dst = a.Preferences().StringWithFallback(profileKey(name, "dst"), `C:\Backup\Android`)
	workers = a.Preferences().IntWithFallback(profileKey(name, "workers"), 5)
	docs = a.Preferences().BoolWithFallback(profileKey(name, "docs"), false)
	exclPresets = a.Preferences().StringWithFallback(profileKey(name, "excl"), "")
	exclCustom = a.Preferences().StringWithFallback(profileKey(name, "excl_custom"), "")
	return
}

// excludePreset defines a human-readable label and the substring pattern it matches.
type excludePreset struct{ label, pattern string }

// exportedProfile is the JSON-serialisable representation of a saved profile (P2).
type exportedProfile struct {
	Name        string `json:"name"`
	Src         string `json:"src"`
	Dst         string `json:"dst"`
	Workers     int    `json:"workers"`
	Docs        bool   `json:"docs"`
	ExclPresets string `json:"excl_presets"`
	ExclCustom  string `json:"excl_custom"`
}

// excludePresets is the list of common one-click exclusion filters.
var excludePresets = []excludePreset{
	{"WhatsApp Status", ".statuses"},
	{"Thumbnails (.thumbnails)", ".thumbnails"},
	{"Temp files (.tmp / .temp)", ".tmp"},
	{"Android app cache (/Android/data/)", "/android/data/"},
	{"Screenshots", "/screenshots/"},
	{"Voice messages", "/voice/"},
	{"Trash", ".trash"},
}

// Media file extensions
var mediaExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true, ".webp": true,
	".heic": true, ".heif": true, ".raw": true, ".cr2": true, ".nef": true, ".arw": true,
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".wmv": true, ".flv": true,
	".3gp": true, ".webm": true, ".m4v": true,
	".mp3": true, ".wav": true, ".flac": true, ".aac": true, ".ogg": true, ".m4a": true,
}

// shouldBackupFile determines if a file should be included in the backup.
func shouldBackupFile(filePath string, includeDocs bool) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	if mediaExtensions[ext] {
		return true
	}
	if includeDocs && sorter.IsDocument(filePath) {
		return true
	}
	return false
}
