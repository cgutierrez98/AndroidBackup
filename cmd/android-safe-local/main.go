package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

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
	a.Settings().SetTheme(&midnightTheme{}) // Apply Custom Theme

	// Load saved language preference
	savedLang := a.Preferences().StringWithFallback("language", "en")
	if savedLang == "es" {
		i18n.SetLanguage(i18n.Spanish)
	} else {
		i18n.SetLanguage(i18n.English)
	}

	t := i18n.T() // Get translations

	w := a.NewWindow(t.AppTitle)
	w.Resize(fyne.NewSize(900, 600)) // Larger default size for dashboard feel

	// -- UI COMPONENTS --

	// 1. Status Section (Sidebar)
	statusLabel := widget.NewLabel(t.CheckingConn)
	statusLabel.Wrapping = fyne.TextWrapWord
	deviceIcon := widget.NewIcon(theme.ComputerIcon()) // Placeholder for phone icon
	statusCard := widget.NewCard(t.DeviceStatus, "", container.NewVBox(
		container.NewHBox(deviceIcon, widget.NewLabel(t.AndroidDevice)),
		statusLabel,
	))

	// 2. Configuration Section (Main Content)
	sourceEntry := widget.NewEntry()
	sourceEntry.SetText("/sdcard/DCIM")

	sourceSelect := widget.NewSelect([]string{
		"/sdcard",
		"/sdcard/DCIM",
		"/sdcard/Download",
		"/sdcard/Pictures",
		"/storage/emulated/0",
	}, func(s string) {
		sourceEntry.SetText(s)
	})
	sourceSelect.PlaceHolder = t.QuickSelect

	destEntry := widget.NewEntry()
	destEntry.SetText("C:\\Backup\\Android")

	// File type options
	includeDocsCheck := widget.NewCheck(t.IncludeDocs, nil)
	includeDocsCheck.SetChecked(false) // Default: only media

	// Language selector
	currentLangSelection := "English"
	if i18n.GetLanguage() == i18n.Spanish {
		currentLangSelection = "Español"
	}

	langSelect := widget.NewSelect([]string{"English", "Español"}, func(selected string) {
		// Only show dialog if actually changed
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
		// Show restart dialog
		dialog.ShowInformation(t.Language,
			"Please restart the application to apply the new language.\n"+
				"Por favor, reinicia la aplicación para aplicar el nuevo idioma.", w)
	})
	// Set current selection without triggering dialog
	langSelect.SetSelected(currentLangSelection)

	configCard := widget.NewCard(t.Configuration, "", container.NewVBox(
		widget.NewLabelWithStyle(t.SourcePath, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, sourceSelect, sourceEntry),
		widget.NewLabelWithStyle(t.DestPath, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		destEntry,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(t.FileTypes, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		includeDocsCheck,
		widget.NewSeparator(),
		widget.NewLabelWithStyle(t.Language, fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		langSelect,
	))

	// 3. LOGS
	logArea := widget.NewMultiLineEntry()
	logArea.SetMinRowsVisible(8)

	// Logger helper
	logPrint := func(msg string) {
		timestamp := time.Now().Format("15:04:05")
		logArea.SetText(logArea.Text + fmt.Sprintf("[%s] %s\n", timestamp, msg))
		logArea.Refresh() // Force redraw
		logArea.CursorRow = len(logArea.Text)
	}

	// 4. Progress
	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	// -- STATE --
	var client *adb.Client
	var files []device_pkg.File

	// -- ACTIONS --
	var scanBtn *widget.Button

	backgroundOp := func(action func()) {
		go func() {
			action()
		}()
	}

	// Scan Action
	scanBtn = widget.NewButtonWithIcon(t.ScanFiles, theme.SearchIcon(), func() {
		if client == nil {
			dialog.ShowError(fmt.Errorf("ADB not initialized"), w)
			return
		}
		logPrint(t.Scanning + " " + sourceEntry.Text + "...")
		scanBtn.Disable()
		progressBar.Show() // Indeterminate or just show it

		backgroundOp(func() {
			defer scanBtn.Enable()
			walker := device_pkg.NewWalker(client)
			var err error
			files, err = walker.Walk(sourceEntry.Text)
			if err != nil {
				logPrint(t.ScanFailed + ": " + err.Error())
				progressBar.Hide()
				return
			}
			logPrint(fmt.Sprintf(t.FoundFiles, len(files)))
			progressBar.Hide()
		})
	})

	// Backup Action
	backupBtn := widget.NewButtonWithIcon(t.StartBackup, theme.DownloadIcon(), func() {
		if len(files) == 0 {
			dialog.ShowInformation("Info", "Please scan for files first.", w)
			return
		}
		logPrint(t.StartingBackup)
		progressBar.SetValue(0)
		progressBar.Show()
		progressBar.Max = float64(len(files))

		backgroundOp(func() {
			// Initialize Registry
			registry := dedup.NewRegistry()
			logPrint(t.LoadingIndex)
			if err := registry.Load(destEntry.Text); err != nil {
				logPrint(t.RegistryWarning + ": " + err.Error())
			}

			agent := &backup.TransferAgent{Client: client}
			pool := backup.NewPool(5, agent, registry)
			pool.Start()

			fileSorter := sorter.NewSorter()
			destRoot := destEntry.Text
			failures := 0
			success := 0

			// Initialize Manifest
			backupManifest := manifest.New()

			// Feeder
			go func() {
				includeDocs := includeDocsCheck.Checked
				for _, f := range files {
					if f.IsDir {
						progressBar.Max = progressBar.Max - 1
						continue
					}

					// Filter by file type
					if !shouldBackupFile(f.Path, includeDocs) {
						progressBar.Max = progressBar.Max - 1
						continue
					}

					relDest := fileSorter.GetDestination(f)
					fullDest := filepath.Join(destRoot, relDest)
					pool.AddJob(backup.Job{
						SourcePath: f.Path,
						DestPath:   fullDest,
						Size:       f.Size,
						Timestamp:  f.Timestamp,
					})
				}
				pool.Close()
			}()

			// Collector
			for res := range pool.Results() {
				if res.Error != nil {
					logPrint(fmt.Sprintf("%s: %s (%v)", t.Fail, filepath.Base(res.Job.SourcePath), res.Error))
					failures++
				} else if res.Skipped {
					logPrint(fmt.Sprintf("%s: %s", t.Skip, filepath.Base(res.Job.SourcePath)))
					success++
				} else {
					// Add to manifest on success
					relPath, _ := filepath.Rel(destRoot, res.Job.DestPath)
					backupManifest.Add(res.Job.SourcePath, relPath, res.Job.Size, res.Job.Timestamp)
					success++
				}
				progressBar.SetValue(progressBar.Value + 1)
			}

			logPrint(fmt.Sprintf(t.Finished, success, failures))

			// Save manifest
			if err := backupManifest.Save(destRoot); err != nil {
				logPrint(t.ManifestSaveFail + ": " + err.Error())
			} else {
				logPrint(t.ManifestSaved)
			}
			progressBar.Hide()
		})
	})

	// Gallery Action
	galleryBtn := widget.NewButtonWithIcon(t.GenerateGallery, theme.MediaPhotoIcon(), func() {
		dest := destEntry.Text
		logPrint(t.GeneratingGallery)
		progressBar.SetValue(0)
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
				}
			}
			progressBar.Hide()
		})
	})

	// Restore Action
	restoreBtn := widget.NewButtonWithIcon(t.Restore, theme.UploadIcon(), func() {
		if client == nil {
			dialog.ShowError(fmt.Errorf("ADB not initialized"), w)
			return
		}
		localPath := destEntry.Text

		// Try to load manifest
		backupManifest, err := manifest.Load(localPath)
		if err != nil {
			// No manifest, fallback to folder push
			remotePath := "/sdcard/Restored"
			cnf := dialog.NewCustomConfirm(
				t.ConfirmRestore,
				t.RestoreNow, t.Cancel,
				widget.NewLabel(fmt.Sprintf(t.NoManifestMsg, remotePath)),
				func(confirmed bool) {
					if !confirmed {
						logPrint(t.RestoreCancelled)
						return
					}
					logPrint(t.RestoringTo + " " + remotePath + "...")
					progressBar.Show()
					backgroundOp(func() {
						err := client.Push(localPath, remotePath)
						if err != nil {
							logPrint(t.RestoreFailed + ": " + err.Error())
						} else {
							logPrint(fmt.Sprintf(t.RestoreComplete, 1, 0) + " " + remotePath)
						}
						progressBar.Hide()
					})
				}, w)
			cnf.Show()
			return
		}

		// Manifest found - restore to original locations
		cnf2 := dialog.NewCustomConfirm(
			t.ConfirmRestore,
			t.RestoreToOriginal, t.Cancel,
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
					// Use parallel restore pool with more workers for small files
					restorePool := backup.NewRestorePool(15, client)
					restorePool.Start()

					total := len(backupManifest.Entries)

					// Feeder goroutine - send all jobs
					go func() {
						for i, entry := range backupManifest.Entries {
							localFile := filepath.Join(localPath, entry.LocalPath)
							restorePool.AddJob(backup.RestoreJob{
								LocalPath:    localFile,
								OriginalPath: entry.OriginalPath,
								Index:        i + 1,
								Total:        total,
							})
						}
						restorePool.Close()
					}()

					// Collector - process results with optimized logging
					success := 0
					failures := 0
					lastLoggedProgress := 0
					logInterval := max(1, total/20) // Log every 5% or at least every file if < 20 files

					for res := range restorePool.Results() {
						if res.Error != nil {
							// Always log failures
							logPrint(fmt.Sprintf("✗ %s: %s - %s", t.Fail, filepath.Base(res.Job.LocalPath), res.Error.Error()))
							failures++
						} else {
							success++
						}

						// Update progress bar always (lightweight)
						processed := success + failures
						progressBar.SetValue(float64(processed))

						// Log progress periodically to avoid UI slowdown
						if processed-lastLoggedProgress >= logInterval || processed == total {
							logPrint(fmt.Sprintf(t.Progress, processed, total))
							lastLoggedProgress = processed
						}
					}
					logPrint(fmt.Sprintf(t.RestoreComplete, success, failures))
					progressBar.Hide()
				})
			}, w)
		cnf2.Show()
	})

	actionsCard := widget.NewCard(t.Actions, "", container.NewGridWithColumns(4,
		scanBtn, backupBtn, galleryBtn, restoreBtn,
	))

	// -- LAYOUT ASSEMBLY --

	// Left Sidebar
	sidebar := container.NewVBox(
		statusCard,
		widget.NewSeparator(),
		// Could add more stats here
	)

	// Right Content
	// Log in accordion
	logItem := widget.NewAccordionItem(t.ActivityLog, logArea)
	logItem.Open = true // Default open for visibility
	logAccordion := widget.NewAccordion(logItem)

	content := container.NewVBox(
		configCard,
		actionsCard,
		progressBar,
		widget.NewSeparator(),
		logAccordion,
	)

	// Use HSplit
	split := container.NewHSplit(sidebar, content)
	split.SetOffset(0.3) // 30% width for sidebar

	w.SetContent(split)

	// -- INITIALIZATION --
	go func() {
		var err error
		client, err = adb.NewClient()
		if err != nil {
			statusLabel.SetText(t.ADBNotFound)
			logPrint(t.ADBError + ": " + err.Error())
			return
		}
		devices, err := client.Devices()
		if err != nil {
			statusLabel.SetText(t.ADBError + ": " + err.Error())
			return
		}
		if len(devices) > 0 {
			statusLabel.SetText(fmt.Sprintf("%s:\n%s\n%s", t.Connected, devices[0].Model, devices[0].Serial))
			statusLabel.TextStyle = fyne.TextStyle{Bold: true}
			logPrint(t.DeviceConnected + ": " + devices[0].Serial)
		} else {
			statusLabel.SetText(t.NoDevice)
			logPrint(t.WaitingDevice)
		}
	}()

	// Cleanup ADB server when window closes
	w.SetOnClosed(func() {
		if client != nil {
			client.KillServer()
		}
	})

	w.ShowAndRun()
}

// Media file extensions (always backed up)
var mediaExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true, ".webp": true,
	".heic": true, ".heif": true, ".raw": true, ".cr2": true, ".nef": true, ".arw": true,
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".wmv": true, ".flv": true,
	".3gp": true, ".webm": true, ".m4v": true,
	".mp3": true, ".wav": true, ".flac": true, ".aac": true, ".ogg": true, ".m4a": true,
}

// Document file extensions (optional)
var documentExtensions = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".txt": true, ".rtf": true, ".odt": true,
	".ods": true, ".odp": true, ".csv": true,
}

// shouldBackupFile determines if a file should be included in the backup
func shouldBackupFile(filePath string, includeDocs bool) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	// Always include media files
	if mediaExtensions[ext] {
		return true
	}

	// Include documents only if checkbox is checked
	if includeDocs && documentExtensions[ext] {
		return true
	}

	return false
}
