package island

import (
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"dynamicisland/internal/preview"
	"dynamicisland/internal/shelf"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

var fileDrop = transfer.DropOptions{
	Formats: []transfer.Format{transfer.FileList, transfer.URIList}, Operations: transfer.Copy,
}

func (a *island) view(c *ui.Context) {
	c.SetTheme(theme)
	c.Root().Background(ui.Transparent)
	root := ui.Column(c.Key("island")).Fill().MinWidth(0).MinHeight(0).Radius(windowRadius).Clip()
	_, over := ui.DataDragOver(root, fileDrop)
	glassSurface(root, c.Vibrancy(), over)
	if drop, ok := ui.DropData(root, fileDrop); ok {
		paths, err := drop.Data.Files()
		if err != nil || len(paths) == 0 {
			a.message("无有效文件", true)
		} else {
			a.addFiles(paths)
		}
	}
	if over && !a.expanded && !a.dragging {
		a.setExpanded(true)
	}
	a.finishMove(c)
	if !a.moving {
		a.hover(c, pointerOverWindow(a.win, root.Hovered()) || over)
	}

	a.updateMedia(c)
	a.updateWave(c)
	if a.expanded && a.emptyIsland() && a.status == "" {
		a.setExpanded(false)
	}
	frame := a.animatedFrame(c)
	w := int(math.Round(float64(frame.width)))
	h := int(math.Round(float64(frame.height)))
	if !a.dragging && !a.moving && (w != a.width || h != a.height) {
		a.resize(w, h)
	}
	a.updateLyrics(c, frame, w, h)
	a.updateIdleInfo(c, frame, w, h)
	root.ContextMenu(a.menu)
	root.Children(func() {
		a.header(c, frame.open)
		if a.expanded || h > compactHeight+1 {
			a.body(c, frame.content)
		}
	})
	a.shortcuts(c)
}

func (a *island) hover(c *ui.Context, hovered bool) {
	if hovered {
		a.leaveSince = time.Time{}
		return
	}
	if !a.expanded || a.dragging || a.media.scrubbing || a.busy > 0 || a.choosing {
		return
	}
	now := c.Now()
	if a.leaveSince.IsZero() {
		a.leaveSince = now
	}
	remaining := 200*time.Millisecond - now.Sub(a.leaveSince)
	if remaining <= 0 {
		a.setExpanded(false)
	} else {
		c.After(remaining)
	}
}

func (a *island) header(c *ui.Context, open float32) {
	height := float32(compactHeight) + 10*open
	docked := a.dock.side != dockNone
	offset := -float32(a.dock.side) * (dockTuck / 2.0) * (1 - open)
	idleFace := open < 1 && a.compactIdle()
	track := a.media.track
	playing := track.Playing && track.Title != ""
	if !idleFace {
		a.face = idleFaceState{}
	}
	header := ui.Row(c.Key("header")).FillWidth().Height(height).Shrink(0).
		Padding(0, 22).Gap(8).AlignItems(ui.Center)
	if a.expanded {
		header.DragWindow().Cursor(ui.CursorGrab).Tooltip("移动")
	} else {
		header.HandleInput(a.capsuleInput).Cursor(ui.CursorGrab)
		press := a.press
		if press != nil && press.released {
			a.press = nil
		}
		if !a.emptyIsland() || a.status != "" {
			header.Cursor(ui.CursorPointer).Focusable().Role(ui.RoleButton).Label("展开灵动岛")
			if header.Clicked() && (press == nil || (press.released && !press.moved)) {
				a.setExpanded(true)
			}
		}
	}
	header.Children(func() {
		if idleFace {
			a.drawIdleFace(c, height, open, offset)
		} else if open < 1 && playing {
			const coverSize, timeWidth = 34, 42
			mediaWidth, waveInset, bandsPerBar := float32(compactMediaWidth-20), float32(51), 4
			if docked {
				mediaWidth, waveInset, bandsPerBar = 18, 0, 13
			} else {
				c.After(200 * time.Millisecond)
			}
			ui.Box(c.Key("compact-media")).Absolute().Left(0).Right(0).Top(0).Height(height).
				Opacity(1-open).Translate(offset, 0).IgnoreInput(true).Center().Children(func() {
				ui.Box(c).Size(mediaWidth, height).Shrink(0).Children(func() {
					ui.Box(c.Key("compact-wave")).Absolute().Left(waveInset).Right(waveInset).
						Top((height - 24) / 2).Height(24).Label("播放律动").
						Draw(func(p *ui.Painter, r ui.Rect) {
							paintMediaBars(p, r, a.media.wave[:], bandsPerBar, 1, a.settings.waveColor(a.media.artworkColor))
						})
					if !docked {
						a.media.cover(c.Key("compact-cover"), coverSize).
							Absolute().Left(0).Top((height - coverSize) / 2)
						ui.Text(c.Key("compact-time"), mediaTime(track.Elapsed(c.Now()))).
							Absolute().Right(0).Top((height-16)/2).Size(timeWidth, 16).
							SingleLine().TextAlign(ui.Center).FontSize(11.5).FontWeight(600).TextColor(ink.Alpha(0.72))
					}
				})
			})
		}
		ui.Box(c.Key("move")).Grow(1)

		if len(a.files.Items) > 0 && !playing {
			summary := ui.Text(c.Key("summary"), fmt.Sprintf("%02d", len(a.files.Items))).
				MinWidth(0).Opacity(1 - open).Clip().
				FontSize(10.5).SingleLine().TextColor(muted)
			if docked {
				summary.Absolute().Left(0).Right(0).Top((height-14)/2).Height(14).
					TextAlign(ui.Center).Translate(offset, 0).IgnoreInput(true)
			} else {
				summary.Width(24 * (1 - open))
			}
		}
	})
}

func (a *island) body(c *ui.Context, opacity float32) {
	ui.Column(c.Key("body")).Width(panelWidth-2).Shrink(0).Padding(0, 22, 20).Gap(14).
		Opacity(opacity).Translate(0, 10*(1-opacity)).IgnoreInput(!a.expanded || opacity < 0.80).Children(func() {
		if a.media.track.Title != "" {
			a.mediaCard(c)
		}
		hasFiles := len(a.files.Items) > 0
		if hasFiles {
			ui.Text(c.Key("files-heading"), fmt.Sprintf("暂存文件  /  %02d", len(a.files.Items))).
				Height(24).Shrink(0).FontSize(10.5).TextColor(muted)
			_, gridHeight := a.panelHeights()
			ui.Scroll(c.Key("files")).Height(float32(gridHeight)).Shrink(0).
				Children(func() {
					ui.Grid(c.Key("file-cards")).FillWidth().Columns(cardColumns).Gap(cardGap).Padding(4).Children(func() {
						for _, item := range slices.Clone(a.files.Items) {
							a.fileCard(c, item)
						}
					})
				})
			ui.Box(c.Key("divider")).FillWidth().Height(1).Shrink(0).
				Gradient(ink.Alpha(0.055), white.Alpha(0.5), 90)
			a.actions(c)
		}
		if hasFiles || a.status != "" {
			color := muted
			if a.statusError {
				color = ui.Hex("#a65b50")
			}
			ui.Text(c.Key("status"), a.status).Height(20).Shrink(0).FontSize(10).SingleLine().TextColor(color).
				Tooltip(a.status)
		}
	})
}

func (a *island) fileCard(c *ui.Context, item shelf.Item) {
	key := shelf.Key(item.Path)
	selected := a.selected[key]
	card := ui.ButtonBase(c.Key(key)).Column().FillWidth().MinWidth(0).Height(cardHeight).Shrink(0).
		Padding(8).Gap(4).AlignItems(ui.Center).Radius(13).
		Gradient(white.Alpha(0.43), white.Alpha(0.17), 155).Border(0.75, white.Alpha(0.65)).
		InsetShadow(0, 1, 0, 0, white.Alpha(0.46)).Shadow(0, 3, 10, -5, ink.Alpha(0.10)).
		Cursor(ui.CursorGrab).Label(item.Name).Tooltip(item.Path)
	hovered := card.Hovered()
	if selected {
		card.Gradient(white.Alpha(0.76), white.Alpha(0.40), 155).Border(1, accent.Alpha(0.42))
	} else if hovered {
		card.Gradient(white.Alpha(0.66), white.Alpha(0.30), 155)
	}
	controlTarget := float32(0)
	if selected || hovered || len(a.selected) > 0 || card.FocusWithin() {
		controlTarget = 1
	}
	controls := card.Animate("controls", controlTarget, 120*time.Millisecond)
	visual := a.previews[key]
	if card.InViewMargin(cardHeight) {
		visual = a.filePreview(item)
	}
	card.DragDataFrom(func() transfer.Data { return a.dragData(item.Path) }, transfer.DragOptions{
		Operations: transfer.Copy, Done: a.dragDone,
	})
	card.ContextMenu(func(m *ui.Menu) {
		if m.Item("复制文件").Chosen() {
			a.copyFiles(a.outgoingPaths(item.Path))
		}
		if m.Item("在文件夹中显示").Chosen() {
			mygo.Shell.ShowItemInFolder(item.Path)
		}
		m.Separator()
		if m.Item("从暂存区移除").Chosen() {
			a.removeFiles([]string{item.Path})
		}
	})
	card.Children(func() {
		icon, color, kind := fileStyle(item)
		ui.Box(c.Key("visual")).FillWidth().Height(48).Shrink(0).Radius(8).Clip().
			Background(white.Alpha(0.16)).Center().Children(func() {
			if visual != nil && visual.source != nil {
				if visual.thumbnail {
					ui.Image(c, visual.source).Fill().Fit(ui.Cover)
				} else {
					ui.Image(c, visual.source).Size(32, 32).Fit(ui.Contain)
				}
			} else {
				ui.Icon(c, icon).Size(28, 28).TextColor(color)
			}
		})
		ui.Text(c, item.Name).FillWidth().MinWidth(0).Height(30).Shrink(0).
			FontSize(11).FontWeight(500).Wrap().MaxLines(2).TextAlign(ui.Center).TextColor(ink)
		info := kind
		if !item.Directory {
			info += " · " + formatSize(item.Size)
		}
		ui.Text(c, info).FillWidth().Height(12).Shrink(0).FontSize(9.5).
			SingleLine().TextAlign(ui.Center).TextColor(muted)
		check := ui.Checkbox(c.Key("select"), &selected, "").Absolute().Left(6).Top(6).
			Padding(1).Radius(5).Background(white.Alpha(0.86)).Opacity(controls).Label("选择 " + item.Name)
		if check.Changed() {
			if selected {
				a.selected[key] = true
			} else {
				delete(a.selected, key)
			}
		}
		if iconButton(c.Key("remove"), closeIcon, "移除").
			Absolute().Right(5).Top(5).Size(20, 20).Background(white.Alpha(0.86)).Opacity(controls).Clicked() {
			a.removeFiles([]string{item.Path})
		}
	})
	if card.Clicked() {
		if card.ClickModifiers()&ui.Cmd == 0 {
			clear(a.selected)
			a.selected[key] = true
		} else if a.selected[key] {
			delete(a.selected, key)
		} else {
			a.selected[key] = true
		}
	}
	if card.DoubleClicked() {
		mygo.Shell.ShowItemInFolder(item.Path)
	}
}

func (a *island) actions(c *ui.Context) {
	paths := a.outgoingPaths("")
	label := "拖出全部"
	if len(a.selected) > 0 {
		label = "拖出所选"
	}
	ui.Row(c.Key("actions")).Height(40).Shrink(0).Gap(8).Children(func() {
		drag := ui.ButtonBase(c.Key("drag-all")).Row().Grow(1).MinWidth(0).Height(40).Gap(8).Center().Radius(13).
			Gradient(white.Alpha(0.7), white.Alpha(0.36), 165).Border(0.75, white.Alpha(0.75)).
			InsetShadow(0, 1, 0, 0, white.Alpha(0.55)).TextColor(ink).
			Cursor(ui.CursorGrab).Label(label).Tooltip(label).
			Disabled(len(paths) == 0)
		if drag.Hovered() {
			drag.Gradient(white.Alpha(0.88), white.Alpha(0.50), 165)
		}
		drag.DragDataFrom(func() transfer.Data { return a.dragData("") }, transfer.DragOptions{
			Operations: transfer.Copy, Done: a.dragDone,
		})
		drag.Children(func() {
			ui.Icon(c, sendIcon).Size(14, 14).TextColor(ink.Alpha(0.72))
			ui.Text(c, label).FontSize(12).SingleLine()
			ui.Text(c, fmt.Sprintf("%02d", len(paths))).FontSize(10).TextColor(muted)
		})
		if iconButton(c.Key("copy"), copyIcon, "复制文件").
			Size(36, 40).Disabled(len(paths) == 0).Clicked() {
			a.copyFiles(paths)
		}
		if iconButton(c.Key("remove-selected"), trashIcon, "移除选中条目").
			Size(36, 40).Disabled(len(a.selected) == 0).Clicked() {
			a.removeFiles(a.selection())
		}
	})
}

func (a *island) menu(m *ui.Menu) {
	if m.Item("添加文件…").Disabled(a.choosing).Chosen() {
		a.choose(false)
	}
	if m.Item("添加文件夹…").Disabled(a.choosing).Chosen() {
		a.choose(true)
	}
	m.Separator()
	if m.Item("全选").Disabled(len(a.files.Items) == 0).Chosen() {
		for _, item := range a.files.Items {
			a.selected[shelf.Key(item.Path)] = true
		}
	}
	if m.Item("取消选择").Disabled(len(a.selected) == 0).Chosen() {
		clear(a.selected)
	}
	if m.Item("清空暂存区").Disabled(len(a.files.Items) == 0).Chosen() {
		a.removeFiles(a.files.Paths())
	}
	m.Separator()
	if m.Item("回到屏幕顶部").Chosen() {
		a.recenter()
	}
	if m.Item("隐藏灵动岛").Chosen() {
		a.hide()
	}
	m.Separator()
	if m.Item("设置…").Chosen() {
		a.win.Update(a.openSettings)
	}
	if m.Item("退出").Chosen() {
		mygo.App.Quit()
	}
}

func (a *island) shortcuts(c *ui.Context) {
	if c.Shortcut(0, ui.KeyEscape) {
		a.setExpanded(false)
	}
	if c.Shortcut(ui.Cmd, ui.KeyA) {
		for _, item := range a.files.Items {
			a.selected[shelf.Key(item.Path)] = true
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyC) && len(a.files.Items) > 0 {
		a.copyFiles(a.outgoingPaths(""))
	}
	if c.Shortcut(ui.Cmd, ui.KeyV) {
		if paths, err := mygo.Clipboard.ReadFiles(); err == nil && len(paths) > 0 {
			a.addFiles(paths)
		} else {
			a.message("剪贴板无文件", true)
		}
	}
	if c.Shortcut(0, ui.KeyDelete) {
		a.removeFiles(a.selection())
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		a.choose(false)
	}
}

func iconButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	button := ui.ButtonBase(c).Size(26, 26).Shrink(0).Radius(8).Center().
		TextColor(muted).Label(label).Tooltip(label).Cursor(ui.CursorPointer)
	if button.Hovered() {
		button.Background(theme.SurfaceHover).TextColor(ink)
	}
	button.Children(func() {
		ui.Icon(c, icon).Size(15, 15)
	})
	return button
}

func fileStyle(item shelf.Item) (*ui.SVG, ui.Color, string) {
	if item.Directory {
		return folderIcon, ui.Hex("#ac9270"), "文件夹"
	}
	if preview.IsImage(item.Name) {
		return imageIcon, ui.Hex("#9483a6"), "图片"
	}
	ext := strings.ToLower(filepath.Ext(item.Name))
	switch ext {
	case ".zip", ".rar", ".7z", ".tar", ".gz":
		return archiveIcon, ui.Hex("#a18b72"), "压缩文件"
	case ".mp4", ".mov", ".mkv", ".avi", ".mp3", ".flac", ".wav":
		return mediaIcon, ui.Hex("#a88494"), "影音文件"
	case ".pdf", ".doc", ".docx", ".ppt", ".pptx", ".xls", ".xlsx":
		return fileIcon, ui.Hex("#7c8ea7"), strings.ToUpper(strings.TrimPrefix(ext, "."))
	default:
		return fileIcon, accent, "文件"
	}
}

func formatSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	for _, unit := range []string{"KB", "MB", "GB", "TB"} {
		value /= 1024
		if value < 1024 || unit == "TB" {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return ""
}
