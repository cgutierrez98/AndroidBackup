package main

import (
	"fmt"
	"path/filepath"
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
	"AndroidSafeLocal/internal/manifest"
	"AndroidSafeLocal/internal/sorter"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(&midnightTheme{}) // Apply Custom Theme

	w := a.NewWindow("AndroidSafeLocal")
	w.Resize(fyne.NewSize(900, 600)) // Larger default size for dashboard feel

	// -- UI COMPONENTS --

	// 1. Status Section (Sidebar)
	statusLabel := widget.NewLabel("Checking connection...")
	statusLabel.Wrapping = fyne.TextWrapWord
	deviceIcon := widget.NewIcon(theme.ComputerIcon()) // Placeholder for phone icon
	statusCard := widget.NewCard("Device Status", "", container.NewVBox(
		container.NewHBox(deviceIcon, widget.NewLabel("Android Device")),
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
	sourceSelect.PlaceHolder = "Quick Select..."

	destEntry := widget.NewEntry()
	destEntry.SetText("C:\\Backup\\Android")

	configCard := widget.NewCard("Configuration", "", container.NewVBox(
		widget.NewLabelWithStyle("Source Path (Mobile)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, sourceSelect, sourceEntry),
		widget.NewLabelWithStyle("Destination Path (PC)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		destEntry,
	))

	// 3. LOGS
	logArea := widget.NewMultiLineEntry()
	logArea.SetMinRowsVisible(8)
	// logArea.Disable() // Enabled for contrast

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
	scanBtn = widget.NewButtonWithIcon("Scan Files", theme.SearchIcon(), func() {
		if client == nil {
			dialog.ShowError(fmt.Errorf("ADB not initialized"), w)
			return
		}
		logPrint("Scanning " + sourceEntry.Text + "...")
		scanBtn.Disable()
		progressBar.Show() // Indeterminate or just show it

		backgroundOp(func() {
			defer scanBtn.Enable()
			walker := device_pkg.NewWalker(client)
			var err error
			files, err = walker.Walk(sourceEntry.Text)
			if err != nil {
				logPrint("Scan failed: " + err.Error())
				progressBar.Hide()
				return
			}
			logPrint(fmt.Sprintf("Found %d files.", len(files)))
			progressBar.Hide()
		})
	})

	// Backup Action
	backupBtn := widget.NewButtonWithIcon("Start Backup", theme.DownloadIcon(), func() {
		if len(files) == 0 {
			dialog.ShowInformation("Info", "Please scan for files first.", w)
			return
		}
		logPrint("Starting backup...")
		progressBar.SetValue(0)
		progressBar.Show()
		progressBar.Max = float64(len(files))

		backgroundOp(func() {
			// Initialize Registry
			registry := dedup.NewRegistry()
			logPrint("Loading local index...")
			if err := registry.Load(destEntry.Text); err != nil {
				logPrint("Registry warning: " + err.Error())
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
				for _, f := range files {
					if f.IsDir {
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
					logPrint(fmt.Sprintf("FAIL: %s (%v)", filepath.Base(res.Job.SourcePath), res.Error))
					failures++
				} else if res.Skipped {
					logPrint(fmt.Sprintf("SKIP: %s", filepath.Base(res.Job.SourcePath)))
					success++
				} else {
					// Add to manifest on success
					relPath, _ := filepath.Rel(destRoot, res.Job.DestPath)
					backupManifest.Add(res.Job.SourcePath, relPath, res.Job.Size, res.Job.Timestamp)
					success++
				}
				progressBar.SetValue(progressBar.Value + 1)
			}

			logPrint(fmt.Sprintf("Finished. Processed: %d. Failures: %d", success, failures))

			// Save manifest
			if err := backupManifest.Save(destRoot); err != nil {
				logPrint("Warning: Failed to save manifest: " + err.Error())
			} else {
				logPrint("Manifest saved.")
			}
			progressBar.Hide()
		})
	})

	// Gallery Action
	galleryBtn := widget.NewButtonWithIcon("Generate Gallery", theme.MediaPhotoIcon(), func() {
		dest := destEntry.Text
		logPrint("Generating Gallery...")
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
					logPrint(fmt.Sprintf("Gallery incomplete (%d items). Error: %s", count, err.Error()))
				} else {
					logPrint("Gallery Error: " + err.Error())
				}
			} else {
				if count == 0 {
					logPrint("No media files found.")
				} else {
					logPrint(fmt.Sprintf("Gallery Created! (%d items)", count))
				}
			}
			progressBar.Hide()
		})
	})

	// Restore Action
	restoreBtn := widget.NewButtonWithIcon("Restore", theme.UploadIcon(), func() {
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
				"Confirm Restore",
				"Restore Now", "Cancel",
				widget.NewLabel(fmt.Sprintf("No manifest found.\nRestore entire folder to:\n%s\n\nExisting files may be overwritten.", remotePath)),
				func(confirmed bool) {
					if !confirmed {
						logPrint("Restore cancelled.")
						return
					}
					logPrint("Restoring folder to " + remotePath + "...")
					progressBar.Show()
					backgroundOp(func() {
						err := client.Push(localPath, remotePath)
						if err != nil {
							logPrint("Restore failed: " + err.Error())
						} else {
							logPrint("Restore Complete! Files are in " + remotePath)
						}
						progressBar.Hide()
					})
				}, w)
			cnf.Show()
			return
		}

		// Manifest found - restore to original locations
		cnf2 := dialog.NewCustomConfirm(
			"Confirm Restore",
			"Restore to Original", "Cancel",
			widget.NewLabel(fmt.Sprintf("Manifest found with %d files.\nRestore each file to its ORIGINAL location on the device?\n\nExisting files with same name will be overwritten.", len(backupManifest.Entries))),
			func(confirmed bool) {
				if !confirmed {
					logPrint("Restore cancelled.")
					return
				}
				logPrint("Restoring to original locations...")
				progressBar.SetValue(0)
				progressBar.Max = float64(len(backupManifest.Entries))
				progressBar.Show()

				backgroundOp(func() {
					success := 0
					failures := 0
					for _, entry := range backupManifest.Entries {
						localFile := filepath.Join(localPath, entry.LocalPath)
						err := client.Push(localFile, entry.OriginalPath)
						if err != nil {
							logPrint(fmt.Sprintf("FAIL: %s", filepath.Base(entry.LocalPath)))
							failures++
						} else {
							success++
						}
						progressBar.SetValue(progressBar.Value + 1)
					}
					logPrint(fmt.Sprintf("Restore Complete. Success: %d, Failures: %d", success, failures))
					progressBar.Hide()
				})
			}, w)
		cnf2.Show()
	})

	actionsCard := widget.NewCard("Actions", "", container.NewGridWithColumns(4,
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
	logItem := widget.NewAccordionItem("Activity Log", logArea)
	logItem.Open = true // Default open? Or closed? Let's leave open for visibility.
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
			statusLabel.SetText("Error: ADB not found")
			logPrint("ADB Error: " + err.Error())
			return
		}
		devices, err := client.Devices()
		if err != nil {
			statusLabel.SetText("ADB Error: " + err.Error())
			return
		}
		if len(devices) > 0 {
			statusLabel.SetText(fmt.Sprintf("Connected:\n%s\n%s", devices[0].Model, devices[0].Serial))
			statusLabel.TextStyle = fyne.TextStyle{Bold: true}
			logPrint("Device connected: " + devices[0].Serial)
		} else {
			statusLabel.SetText("No Device Connected.\nCheck USB Cable.")
			logPrint("Waiting for device...")
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
