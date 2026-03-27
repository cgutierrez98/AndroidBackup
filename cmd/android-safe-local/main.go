package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	logArea.Disable() // Read-only: prevents accidental edits by the user

	// Logger helper (mutex-protected: logArea.Text is read-written from background goroutines)
	var logMu sync.Mutex
	logPrint := func(msg string) {
		logMu.Lock()
		defer logMu.Unlock()
		timestamp := time.Now().Format("15:04:05")
		logArea.SetText(logArea.Text + fmt.Sprintf("[%s] %s\n", timestamp, msg))
		logArea.Refresh() // Force redraw
	}

	// 4. Progress
	progressBar := widget.NewProgressBar()
	progressBar.Hide()

	// -- STATE --
	// stateMu guards client and files, which are written/read from multiple goroutines.
	var stateMu sync.RWMutex
	var client *adb.Client
	var files []device_pkg.File

	// -- ACTIONS --
	var scanBtn *widget.Button

	// setActionsEnabled enables or disables all action buttons atomically to prevent
	// concurrent overlapping operations.
	var backupBtn, galleryBtn, restoreBtn *widget.Button
	setActionsEnabled := func(enabled bool) {
		for _, btn := range []*widget.Button{scanBtn, backupBtn, galleryBtn, restoreBtn} {
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

	// Scan Action
	scanBtn = widget.NewButtonWithIcon(t.ScanFiles, theme.SearchIcon(), func() {
		stateMu.RLock()
		c := client
		stateMu.RUnlock()
		if c == nil {
			dialog.ShowError(errors.New("ADB not initialized"), w)
			return
		}
		logPrint(t.Scanning + " " + sourceEntry.Text + "...")
		progressBar.Show() // Indeterminate or just show it

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
		})
	})

	// Backup Action
	backupBtn = widget.NewButtonWithIcon(t.StartBackup, theme.DownloadIcon(), func() {
		stateMu.RLock()
		localFiles := append([]device_pkg.File(nil), files...)
		stateMu.RUnlock()

		if len(localFiles) == 0 {
			dialog.ShowInformation(t.InfoTitle, t.ScanFirst, w)
			return
		}

		// Count eligible files upfront: avoids mutating progressBar.Max from a goroutine
		includeDocs := includeDocsCheck.Checked
		eligibleCount := 0
		for _, f := range localFiles {
			if !f.IsDir && shouldBackupFile(f.Path, includeDocs) {
				eligibleCount++
			}
		}

		destRoot := destEntry.Text

		stateMu.RLock()
		c := client
		stateMu.RUnlock()

		// startBackup contains the actual backup logic. resume=true if the user
		// chose to continue a previously interrupted session.
		startBackup := func(resume bool) {
			logPrint(t.StartingBackup)
			progressBar.SetValue(0)
			progressBar.Max = float64(eligibleCount)
			progressBar.Show()

			backgroundOp(func() {
				// Load resume state if requested
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

				// Initialize Registry
				registry := dedup.NewRegistry()
				logPrint(t.LoadingIndex)
				// Load existing manifest hashes first (faster than filesystem walk for large backups)
				if existingManifest, err := manifest.Load(destRoot); err == nil {
					registry.LoadHashes(existingManifest.HashSet())
				}
				if err := registry.Load(destRoot); err != nil {
					logPrint(t.RegistryWarning + ": " + err.Error())
				}

				agent := &backup.TransferAgent{Client: c}
				pool := backup.NewPool(5, agent, registry)
				pool.Start()

				fileSorter := sorter.NewSorter()
				failures := 0
				success := 0

				// Initialize Manifest
				backupManifest := manifest.New()

				// Feeder
				go func() {
					for _, f := range localFiles {
						if f.IsDir {
							continue
						}
						if !shouldBackupFile(f.Path, includeDocs) {
							continue
						}
						// Skip paths already completed in the previous session
						if completedPaths[f.Path] {
							continue
						}
						var relDest string
						if includeDocs && sorter.IsDocument(f.Path) {
							relDest = fileSorter.GetDocumentDestination(f, sourceEntry.Text)
						} else {
							relDest = fileSorter.GetDestination(f)
						}
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
				processed := 0
				for res := range pool.Results() {
					if res.Error != nil {
						logPrint(fmt.Sprintf("%s: %s (%v)", t.Fail, filepath.Base(res.Job.SourcePath), res.Error))
						failures++
					} else if res.Skipped {
						logPrint(fmt.Sprintf("%s: %s", t.Skip, filepath.Base(res.Job.SourcePath)))
						success++
					} else {
						// P4: Re-sort using EXIF DateTimeOriginal when available
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
						// Compute xxHash of the local file for integrity tracking
						hash, _ := xxhashFile(actualDest)
						if hash != "" {
							registry.AddByHash(hash)
						}
						// Record completion in state file (persist every 10 files)
						runState.CompletedPaths[res.Job.SourcePath] = true
						if len(runState.CompletedPaths)%10 == 0 {
							_ = runState.Save(destRoot)
						}
						// Add to manifest on success
						relPath, _ := filepath.Rel(destRoot, actualDest)
						backupManifest.Add(res.Job.SourcePath, relPath, res.Job.Size, res.Job.Timestamp, hash)
						success++
					}
					processed++
					progressBar.SetValue(float64(processed))
				}

				logPrint(fmt.Sprintf(t.Finished, success, failures))

				// Save manifest
				if err := backupManifest.Save(destRoot); err != nil {
					logPrint(t.ManifestSaveFail + ": " + err.Error())
				} else {
					logPrint(t.ManifestSaved)
				}

				// Clear resume state on successful completion
				_ = backup.ClearState(destRoot)
				progressBar.Hide()
			})
		}

		// If an interrupted session exists, ask the user whether to resume it
		if backup.StateExists(destRoot) {
			dialog.NewCustomConfirm(
				t.ResumeTitle, t.ResumeYes, t.ResumeNo,
				widget.NewLabel(t.ResumeMsg),
				func(resume bool) { startBackup(resume) },
				w,
			).Show()
		} else {
			startBackup(false)
		}
	})

	// Gallery Action
	galleryBtn = widget.NewButtonWithIcon(t.GenerateGallery, theme.MediaPhotoIcon(), func() {
		dest := destEntry.Text
		logPrint(t.GeneratingGallery)
		progressBar.SetValue(0)
		progressBar.Max = 1 // will be updated by callback on first tick
		progressBar.Show()

		backgroundOp(func() {
			gen := gallery.NewGenerator()
			count, err := gen.Generate(dest, func(current, total int) {
				// Set Max once (total is stable after first call) then update value.
				// Both writes happen on this single goroutine, so no concurrent race.
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
	restoreBtn = widget.NewButtonWithIcon(t.Restore, theme.UploadIcon(), func() {
		stateMu.RLock()
		c := client
		stateMu.RUnlock()
		if c == nil {
			dialog.ShowError(errors.New("ADB not initialized"), w)
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
						err := c.Push(localPath, remotePath)
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
					restorePool := backup.NewRestorePool(15, c)
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
		newClient, err := adb.NewClient()
		if err != nil {
			statusLabel.SetText(t.ADBNotFound)
			logPrint(t.ADBError + ": " + err.Error())
			return
		}
		stateMu.Lock()
		client = newClient
		stateMu.Unlock()
		devices, err := newClient.Devices()
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
// Returns "" on any error so callers can treat it as "hash unknown".
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

// Media file extensions (always backed up)
var mediaExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true, ".webp": true,
	".heic": true, ".heif": true, ".raw": true, ".cr2": true, ".nef": true, ".arw": true,
	".mp4": true, ".mov": true, ".avi": true, ".mkv": true, ".wmv": true, ".flv": true,
	".3gp": true, ".webm": true, ".m4v": true,
	".mp3": true, ".wav": true, ".flac": true, ".aac": true, ".ogg": true, ".m4a": true,
}

// shouldBackupFile determines if a file should be included in the backup.
// Media files are always included; documents only when includeDocs is true.
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
