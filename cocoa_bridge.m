/*
 * cocoa_bridge.m -- AppKit UI implementation for FrameScope.
 *
 * This file owns the entire native macOS UI: window, toolbar, split view, two
 * table views, and the status bar. It communicates with the Go layer through
 * the C functions declared in cocoa_bridge.h:
 *
 *  Go -> Cocoa  UpdateResults / ShowErrorMessage  (called from ui_bridge.go)
 *  Cocoa -> Go  GoStartMonitoring, GoStopMonitoring, GoSet*, GoSelectFrame
 *               (called in response to user actions)
 *
 * All UI mutations must happen on the main thread. Go calls UpdateResults and
 * ShowErrorMessage from arbitrary goroutines; both functions dispatch their
 * work asynchronously onto the main queue before touching any AppKit objects.
 *
 * Layout overview (top-to-bottom inside the content view):
 *
 *   +-------------------------------------+  <- NSToolbar (managed by AppKit)
 *   |  Frame (s): [__]  [Start] [Stop]    |    RecordingItem
 *   |  [< Prev] [popup] [Next >]          |    NavigationItem
 *   |                          [Settings] |    OptionsItem
 *   +-------------------------------------+
 *   |  Current Frame         (header 22px)|
 *   |  NSTableView (resultsTable)         |  <- frame pane (~58%)
 *   |  PID | Raw(s) | CPU Time | Command  |
 *   +- - - - - - - - - - - - - - - - - - +  <- NSSplitView thin divider
 *   |  Summary -- Totals & Averages       |
 *   |  NSTableView (summaryTable)         |  <- summary pane (~42%)
 *   |  PID | Total | Avg | ... | Command  |
 *   +-------------------------------------+
 *   |  status text                   24px |  <- status bar (fixed, bottom)
 *   +-------------------------------------+
 */

#import <Cocoa/Cocoa.h>
#include <unistd.h>
#include <stdlib.h>
#include "cocoa_bridge.h"

/** Version string set by SetAppVersion() before RunApp() is called. */
static NSString *gAppVersion = @"dev";

/* Toolbar item identifier constants. */
static NSString * const kRecordingItem  = @"RecordingItem";
static NSString * const kNavigationItem = @"NavigationItem";
static NSString * const kOptionsItem    = @"OptionsItem";

/**
 * MonitorAppDelegate is the single NSApplicationDelegate for FrameScope.
 * It also acts as NSTableViewDataSource and NSTableViewDelegate for both
 * table views, and as NSToolbarDelegate for the main toolbar.
 *
 * Table data is stored as pre-parsed arrays of string arrays (frameRows /
 * summaryRows) populated by applyRowsPayload: / applySummaryPayload: whenever
 * Go pushes a new update. The delegate methods simply index into these arrays.
 */
@interface MonitorAppDelegate : NSObject <NSApplicationDelegate,
                                          NSTableViewDataSource,
                                          NSTableViewDelegate,
                                          NSToolbarDelegate>

/* Main window. */
@property(nonatomic, strong) NSWindow      *window;

/* RecordingItem controls. */
@property(nonatomic, strong) NSTextField   *frameField;
@property(nonatomic, strong) NSButton      *startButton;
@property(nonatomic, strong) NSButton      *stopButton;

/* OptionsItem controls. */
@property(nonatomic, strong) NSButton      *settingsButton;
@property(nonatomic, strong) NSMenuItem    *hideSmallMenuItem;
@property(nonatomic, strong) NSMenuItem    *hidePathsMenuItem;

/* NavigationItem controls. */
@property(nonatomic, strong) NSPopUpButton *historyPopup;
@property(nonatomic, strong) NSButton      *previousFrameButton;
@property(nonatomic, strong) NSButton      *nextFrameButton;

/* Status bar label (bottom of content view). */
@property(nonatomic, strong) NSTextField   *statusLabel;

/* Frame pane (top split). */
@property(nonatomic, strong) NSScrollView  *tableScrollView;
@property(nonatomic, strong) NSTableView   *resultsTable;
@property(nonatomic, strong) NSTextField   *emptyLabel;       /* shown when no rows */

/* Summary pane (bottom split). */
@property(nonatomic, strong) NSScrollView  *summaryScrollView;
@property(nonatomic, strong) NSTableView   *summaryTable;
@property(nonatomic, strong) NSTextField   *summaryEmptyLabel;

/* Table data — arrays of column-value arrays, indexed by row. */
@property(nonatomic, copy) NSArray<NSArray<NSString *> *> *frameRows;
@property(nonatomic, copy) NSArray<NSArray<NSString *> *> *summaryRows;

/* Frame labels displayed in the history popup. */
@property(nonatomic, copy) NSArray<NSString *> *historyItems;

/**
 * Guard flag set to YES while programmatically updating historyPopup selection
 * to prevent re-entrant calls back into GoSelectFrame.
 */
@property(nonatomic, assign) BOOL updatingHistorySelection;

@end

@implementation MonitorAppDelegate

/**
 * applyAppIcon loads FrameScopeIcon.png from the working directory and sets it
 * as the application Dock icon. Silently skipped if the file is not found
 * (e.g. when running from a built .app bundle that uses the .icns resource).
 */
- (void)applyAppIcon {
    NSString *cwd = [[NSFileManager defaultManager] currentDirectoryPath];
    NSString *iconPath = [cwd stringByAppendingPathComponent:@"FrameScopeIcon.png"];
    NSImage *icon = [[NSImage alloc] initWithContentsOfFile:iconPath];
    if (icon != nil) {
        icon.size = NSMakeSize(512, 512);
        [NSApp setApplicationIconImage:icon];
    }
}

#pragma mark - NSToolbarDelegate

/** Returns all item identifiers the toolbar is allowed to contain. */
- (NSArray<NSToolbarItemIdentifier> *)toolbarAllowedItemIdentifiers:(NSToolbar *)toolbar {
    (void)toolbar;
    return @[kRecordingItem, kNavigationItem, kOptionsItem, NSToolbarFlexibleSpaceItemIdentifier];
}

/** Returns the default left-to-right toolbar layout. */
- (NSArray<NSToolbarItemIdentifier> *)toolbarDefaultItemIdentifiers:(NSToolbar *)toolbar {
    (void)toolbar;
    return @[kRecordingItem, NSToolbarFlexibleSpaceItemIdentifier,
             kNavigationItem, NSToolbarFlexibleSpaceItemIdentifier,
             kOptionsItem];
}

/**
 * Constructs and returns the NSToolbarItem for the given identifier.
 *
 * RecordingItem  — "Frame (s):" label + text field + Start + Stop buttons.
 * NavigationItem — ‹ Prev button + history popup + Next › button.
 * OptionsItem    — Settings button with a drop-down menu containing the two
 *                  filter toggles (Hide <1s, Show basenames only).
 *
 * minSize/maxSize are set to fix each item's width. The deprecation warning for
 * those properties is suppressed; they remain the only reliable way to constrain
 * custom-view toolbar items across all supported macOS versions.
 */
- (NSToolbarItem *)toolbar:(NSToolbar *)toolbar
     itemForItemIdentifier:(NSToolbarItemIdentifier)identifier
 willBeInsertedIntoToolbar:(BOOL)flag {
    (void)toolbar; (void)flag;
    NSToolbarItem *item = [[NSToolbarItem alloc] initWithItemIdentifier:identifier];
    item.autovalidates = NO;

    if ([identifier isEqualToString:kRecordingItem]) {
        // [ Frame (s): [5____] [  Start  ] [  Stop  ] ]
        NSView *c = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 308, 32)];

        NSTextField *lbl = [self makeLabel:@"Frame (s):" frame:NSMakeRect(0, 8, 70, 17)];
        lbl.font = [NSFont systemFontOfSize:12];
        lbl.alignment = NSTextAlignmentRight;
        [c addSubview:lbl];

        self.frameField = [[NSTextField alloc] initWithFrame:NSMakeRect(74, 5, 54, 24)];
        self.frameField.stringValue = [NSString stringWithFormat:@"%.0f", GoInitialFrameSeconds()];
        self.frameField.font = [NSFont systemFontOfSize:13];
        [c addSubview:self.frameField];

        self.startButton = [[NSButton alloc] initWithFrame:NSMakeRect(136, 3, 82, 28)];
        self.startButton.title = @"Start";
        self.startButton.bezelStyle = NSBezelStyleRounded;
        self.startButton.target = self;
        self.startButton.action = @selector(startPressed:);
        [c addSubview:self.startButton];

        self.stopButton = [[NSButton alloc] initWithFrame:NSMakeRect(224, 3, 80, 28)];
        self.stopButton.title = @"Stop";
        self.stopButton.bezelStyle = NSBezelStyleRounded;
        self.stopButton.target = self;
        self.stopButton.action = @selector(stopPressed:);
        [c addSubview:self.stopButton];

        item.view = c;
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        item.minSize = NSMakeSize(308, 32);
        item.maxSize = NSMakeSize(308, 32);
#pragma clang diagnostic pop
        return item;
    }

    if ([identifier isEqualToString:kNavigationItem]) {
        // [ ‹ Prev ] [ popup ] [ Next › ]
        NSView *c = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 368, 32)];

        self.previousFrameButton = [[NSButton alloc] initWithFrame:NSMakeRect(0, 3, 76, 28)];
        self.previousFrameButton.title = @"‹ Prev";
        self.previousFrameButton.bezelStyle = NSBezelStyleRounded;
        self.previousFrameButton.target = self;
        self.previousFrameButton.action = @selector(previousFrame:);
        self.previousFrameButton.enabled = NO;
        [c addSubview:self.previousFrameButton];

        self.historyPopup = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(82, 3, 212, 28) pullsDown:NO];
        self.historyPopup.target = self;
        self.historyPopup.action = @selector(historyChanged:);
        self.historyPopup.enabled = NO;
        [self.historyPopup addItemWithTitle:@"No frames yet"];
        [c addSubview:self.historyPopup];

        self.nextFrameButton = [[NSButton alloc] initWithFrame:NSMakeRect(300, 3, 68, 28)];
        self.nextFrameButton.title = @"Next ›";
        self.nextFrameButton.bezelStyle = NSBezelStyleRounded;
        self.nextFrameButton.target = self;
        self.nextFrameButton.action = @selector(nextFrame:);
        self.nextFrameButton.enabled = NO;
        [c addSubview:self.nextFrameButton];

        item.view = c;
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        item.minSize = NSMakeSize(368, 32);
        item.maxSize = NSMakeSize(368, 32);
#pragma clang diagnostic pop
        return item;
    }

    if ([identifier isEqualToString:kOptionsItem]) {
        self.settingsButton = [[NSButton alloc] initWithFrame:NSMakeRect(0, 0, 96, 28)];
        self.settingsButton.title = @"Settings";
        self.settingsButton.bezelStyle = NSBezelStyleRounded;
        self.settingsButton.target = self;
        self.settingsButton.action = @selector(showSettingsMenu:);

        NSMenu *menu = [[NSMenu alloc] initWithTitle:@"Settings"];

        self.hideSmallMenuItem = [[NSMenuItem alloc] initWithTitle:@"Hide processes below 1s"
                                                            action:@selector(hideSmallToggled:)
                                                     keyEquivalent:@""];
        self.hideSmallMenuItem.target = self;
        self.hideSmallMenuItem.state = GoInitialHideSmall() ? NSControlStateValueOn : NSControlStateValueOff;
        [menu addItem:self.hideSmallMenuItem];

        self.hidePathsMenuItem = [[NSMenuItem alloc] initWithTitle:@"Show basenames only"
                                                            action:@selector(hidePathsToggled:)
                                                     keyEquivalent:@""];
        self.hidePathsMenuItem.target = self;
        self.hidePathsMenuItem.state = GoInitialHidePaths() ? NSControlStateValueOn : NSControlStateValueOff;
        [menu addItem:self.hidePathsMenuItem];

        item.view = self.settingsButton;
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        item.minSize = NSMakeSize(96, 28);
        item.maxSize = NSMakeSize(96, 28);
#pragma clang diagnostic pop
        self.settingsButton.menu = menu;
        return item;
    }

    return item;
}

#pragma mark - Application Lifecycle

/**
 * Builds the entire window hierarchy programmatically:
 *  1. Creates and centres the main window with the version in its title.
 *  2. Attaches the NSToolbar.
 *  3. Pins a 24 pt status bar to the bottom of the content view.
 *  4. Fills the remaining space with an NSSplitView containing the frame pane
 *     (top, ~58 %) and the summary pane (bottom, ~42 %).
 *  5. Configures both NSTableViews with their columns and delegates.
 *  6. Makes the window key and starts monitoring immediately with the last
 *     saved frame length.
 */
- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    (void)notification;

    self.window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 1080, 680)
                                              styleMask:(NSWindowStyleMaskTitled |
                                                         NSWindowStyleMaskClosable |
                                                         NSWindowStyleMaskMiniaturizable |
                                                         NSWindowStyleMaskResizable)
                                                backing:NSBackingStoreBuffered
                                                  defer:NO];
    [self.window setTitle:[NSString stringWithFormat:@"FrameScope %@", gAppVersion]];
    [self.window center];
    [self.window setMinSize:NSMakeSize(700, 480)];

    NSToolbar *toolbar = [[NSToolbar alloc] initWithIdentifier:@"MainToolbar"];
    toolbar.delegate = self;
    toolbar.allowsUserCustomization = NO;
    toolbar.autosavesConfiguration = NO;
    self.window.toolbar = toolbar;

    NSView *content = self.window.contentView;
    content.autoresizesSubviews = YES;
    CGFloat W = content.bounds.size.width;
    CGFloat H = content.bounds.size.height;

    // ── Status bar pinned to bottom ───────────────────────────────────────────
    CGFloat statusH = 24;
    NSView *statusBar = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, W, statusH)];
    statusBar.autoresizingMask = NSViewWidthSizable | NSViewMaxYMargin;

    NSBox *statusSep = [[NSBox alloc] initWithFrame:NSMakeRect(0, statusH - 1, W, 1)];
    statusSep.boxType = NSBoxSeparator;
    statusSep.autoresizingMask = NSViewWidthSizable | NSViewMinYMargin;
    [statusBar addSubview:statusSep];

    self.statusLabel = [self makeLabel:@"Idle. Set a frame length and press Start."
                                 frame:NSMakeRect(10, 5, W - 20, 15)];
    self.statusLabel.font = [NSFont systemFontOfSize:11];
    self.statusLabel.textColor = [NSColor secondaryLabelColor];
    self.statusLabel.autoresizingMask = NSViewWidthSizable;
    [statusBar addSubview:self.statusLabel];
    [content addSubview:statusBar];

    // ── NSSplitView (fills everything above the status bar) ──────────────────
    NSSplitView *split = [[NSSplitView alloc] initWithFrame:NSMakeRect(0, statusH, W, H - statusH)];
    split.vertical = NO;   // horizontal divider → top/bottom panes
    split.dividerStyle = NSSplitViewDividerStyleThin;
    split.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

    CGFloat headerH   = 22;
    CGFloat framePaneH   = (H - statusH) * 0.58;
    CGFloat summaryPaneH = (H - statusH) - framePaneH;

    // ── Frame pane (top) ─────────────────────────────────────────────────────
    NSView *framePane = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, W, framePaneH)];
    framePane.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

    NSView *frameHeader = [self makeSectionHeader:@"Current Frame" width:W];
    frameHeader.frame = NSMakeRect(0, framePaneH - headerH, W, headerH);
    frameHeader.autoresizingMask = NSViewWidthSizable | NSViewMinYMargin;
    [framePane addSubview:frameHeader];

    self.tableScrollView = [[NSScrollView alloc] initWithFrame:NSMakeRect(0, 0, W, framePaneH - headerH)];
    self.tableScrollView.hasVerticalScroller = YES;
    self.tableScrollView.hasHorizontalScroller = YES;
    self.tableScrollView.autohidesScrollers = YES;
    self.tableScrollView.borderType = NSNoBorder;
    self.tableScrollView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

    self.resultsTable = [[NSTableView alloc] initWithFrame:self.tableScrollView.bounds];
    self.resultsTable.usesAlternatingRowBackgroundColors = YES;
    self.resultsTable.allowsColumnResizing = YES;
    self.resultsTable.allowsTypeSelect = YES;
    self.resultsTable.rowSizeStyle = NSTableViewRowSizeStyleDefault;
    self.resultsTable.gridStyleMask = NSTableViewSolidVerticalGridLineMask;
    self.resultsTable.dataSource = self;
    self.resultsTable.delegate = self;
    [self.resultsTable addTableColumn:[self columnWithID:@"pid"     title:@"PID"      width:80  minWidth:60]];
    [self.resultsTable addTableColumn:[self columnWithID:@"raw"     title:@"Raw (s)"  width:82  minWidth:60]];
    [self.resultsTable addTableColumn:[self columnWithID:@"cpu"     title:@"CPU Time" width:110 minWidth:90]];
    NSTableColumn *cmdCol = [self columnWithID:@"command" title:@"Command" width:700 minWidth:200];
    cmdCol.resizingMask = NSTableColumnAutoresizingMask | NSTableColumnUserResizingMask;
    [self.resultsTable addTableColumn:cmdCol];
    self.tableScrollView.documentView = self.resultsTable;
    [framePane addSubview:self.tableScrollView];

    self.emptyLabel = [self makeLabel:@"Press Start to begin."
                                frame:NSMakeRect(14, 44, 200, 18)];
    self.emptyLabel.font = [NSFont systemFontOfSize:12];
    self.emptyLabel.textColor = [NSColor tertiaryLabelColor];
    self.emptyLabel.autoresizingMask = NSViewMaxXMargin | NSViewMaxYMargin;
    [framePane addSubview:self.emptyLabel];

    [split addSubview:framePane];

    // ── Summary pane (bottom) ─────────────────────────────────────────────────
    NSView *summaryPane = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, W, summaryPaneH)];
    summaryPane.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

    NSView *summaryHeader = [self makeSectionHeader:@"Summary — Totals & Averages" width:W];
    summaryHeader.frame = NSMakeRect(0, summaryPaneH - headerH, W, headerH);
    summaryHeader.autoresizingMask = NSViewWidthSizable | NSViewMinYMargin;
    [summaryPane addSubview:summaryHeader];

    self.summaryScrollView = [[NSScrollView alloc] initWithFrame:NSMakeRect(0, 0, W, summaryPaneH - headerH)];
    self.summaryScrollView.hasVerticalScroller = YES;
    self.summaryScrollView.hasHorizontalScroller = YES;
    self.summaryScrollView.autohidesScrollers = YES;
    self.summaryScrollView.borderType = NSNoBorder;
    self.summaryScrollView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

    self.summaryTable = [[NSTableView alloc] initWithFrame:self.summaryScrollView.bounds];
    self.summaryTable.usesAlternatingRowBackgroundColors = YES;
    self.summaryTable.allowsColumnResizing = YES;
    self.summaryTable.allowsTypeSelect = YES;
    self.summaryTable.rowSizeStyle = NSTableViewRowSizeStyleDefault;
    self.summaryTable.gridStyleMask = NSTableViewSolidVerticalGridLineMask;
    self.summaryTable.dataSource = self;
    self.summaryTable.delegate = self;
    [self.summaryTable addTableColumn:[self columnWithID:@"sum_pid"       title:@"PID"       width:80  minWidth:60]];
    [self.summaryTable addTableColumn:[self columnWithID:@"sum_total"     title:@"Total (s)" width:82  minWidth:60]];
    [self.summaryTable addTableColumn:[self columnWithID:@"sum_avg"       title:@"Avg (s)"   width:78  minWidth:60]];
    [self.summaryTable addTableColumn:[self columnWithID:@"sum_total_cpu" title:@"Total CPU" width:100 minWidth:80]];
    [self.summaryTable addTableColumn:[self columnWithID:@"sum_avg_cpu"   title:@"Avg CPU"   width:100 minWidth:80]];
    NSTableColumn *sumCmdCol = [self columnWithID:@"sum_command" title:@"Command" width:530 minWidth:180];
    sumCmdCol.resizingMask = NSTableColumnAutoresizingMask | NSTableColumnUserResizingMask;
    [self.summaryTable addTableColumn:sumCmdCol];
    self.summaryScrollView.documentView = self.summaryTable;
    [summaryPane addSubview:self.summaryScrollView];

    self.summaryEmptyLabel = [self makeLabel:@"Completed frames will appear here."
                                       frame:NSMakeRect(14, 44, 240, 18)];
    self.summaryEmptyLabel.font = [NSFont systemFontOfSize:12];
    self.summaryEmptyLabel.textColor = [NSColor tertiaryLabelColor];
    self.summaryEmptyLabel.autoresizingMask = NSViewMaxXMargin | NSViewMaxYMargin;
    [summaryPane addSubview:self.summaryEmptyLabel];

    [split addSubview:summaryPane];
    [content addSubview:split];

    self.frameRows    = @[];
    self.summaryRows  = @[];
    self.historyItems = @[];
    [self refreshEmptyState];
    [self refreshHistoryControls];
    [self applyAppIcon];

    [self.window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
    // Defer the initial monitoring start until after the run loop is active so
    // the first UI push lands on an already-running main queue.
    dispatch_async(dispatch_get_main_queue(), ^{
        GoStartMonitoring(self.frameField.doubleValue);
    });
}

/** Stops monitoring and terminates the app when the last window is closed. */
- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
    (void)sender;
    GoStopMonitoring();
    return YES;
}

#pragma mark - Actions

/** Reads the current frame-field value and starts a new monitoring run. */
- (void)startPressed:(id)sender {
    (void)sender;
    GoStartMonitoring(self.frameField.doubleValue);
}

/** Stops the active monitoring run. */
- (void)stopPressed:(id)sender {
    (void)sender;
    GoStopMonitoring();
}

/**
 * Toggles the "Hide processes below 1s" menu item state and propagates the
 * change to Go.
 */
- (void)hideSmallToggled:(id)sender {
    (void)sender;
    self.hideSmallMenuItem.state =
        (self.hideSmallMenuItem.state == NSControlStateValueOn) ? NSControlStateValueOff : NSControlStateValueOn;
    GoSetHideSmall(self.hideSmallMenuItem.state == NSControlStateValueOn ? 1 : 0);
}

/**
 * Toggles the "Show basenames only" menu item state and propagates the change
 * to Go.
 */
- (void)hidePathsToggled:(id)sender {
    (void)sender;
    self.hidePathsMenuItem.state =
        (self.hidePathsMenuItem.state == NSControlStateValueOn) ? NSControlStateValueOff : NSControlStateValueOn;
    GoSetHidePaths(self.hidePathsMenuItem.state == NSControlStateValueOn ? 1 : 0);
}

/** Pops the Settings drop-down menu directly below the Settings button. */
- (void)showSettingsMenu:(id)sender {
    NSButton *button = (NSButton *)sender;
    if (button.menu == nil) return;
    [button.menu popUpMenuPositioningItem:nil
                               atLocation:NSMakePoint(0, NSHeight(button.bounds) + 4)
                                   inView:button];
}

/**
 * Called when the user picks an item in the history popup. The guard flag
 * updatingHistorySelection prevents this from firing during programmatic
 * selection changes made by applyHistoryPayload:.
 */
- (void)historyChanged:(id)sender {
    (void)sender;
    if (self.updatingHistorySelection) return;
    GoSelectFrame((int)self.historyPopup.indexOfSelectedItem);
}

/** Moves selection one step earlier in the history popup. */
- (void)previousFrame:(id)sender {
    (void)sender;
    NSInteger index = self.historyPopup.indexOfSelectedItem;
    if (index > 0) {
        [self.historyPopup selectItemAtIndex:index - 1];
        [self historyChanged:self.historyPopup];
    }
}

/** Moves selection one step later in the history popup. */
- (void)nextFrame:(id)sender {
    (void)sender;
    NSInteger index = self.historyPopup.indexOfSelectedItem;
    if (index >= 0 && index + 1 < self.historyPopup.numberOfItems) {
        [self.historyPopup selectItemAtIndex:index + 1];
        [self historyChanged:self.historyPopup];
    }
}

#pragma mark - NSTableViewDataSource / Delegate

/** Returns the number of rows for the given table view. */
- (NSInteger)numberOfRowsInTableView:(NSTableView *)tableView {
    return (tableView == self.summaryTable)
        ? (NSInteger)self.summaryRows.count
        : (NSInteger)self.frameRows.count;
}

/**
 * Returns a cell view for the given table/column/row combination. Cells are
 * reused from the table's view pool. Each cell is a non-editable, selectable
 * NSTextField using the system monospaced font at 12 pt. The full cell value
 * is also set as the tooltip so truncated content remains readable.
 */
- (nullable NSView *)tableView:(NSTableView *)tableView
            viewForTableColumn:(NSTableColumn *)tableColumn
                           row:(NSInteger)row {
    NSString *identifier = tableColumn.identifier;
    NSTextField *cell = [tableView makeViewWithIdentifier:identifier owner:self];
    if (!cell) {
        cell = [[NSTextField alloc] initWithFrame:NSZeroRect];
        cell.identifier = identifier;
        cell.bezeled = NO;
        cell.drawsBackground = NO;
        cell.editable = NO;
        cell.selectable = YES;
        cell.lineBreakMode = NSLineBreakByTruncatingMiddle;
        cell.font = [NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightRegular];
    }
    NSArray<NSArray<NSString *> *> *rows = (tableView == self.summaryTable)
        ? self.summaryRows : self.frameRows;
    NSArray<NSString *> *rowValues = rows[(NSUInteger)row];
    NSUInteger col = [tableView.tableColumns indexOfObject:tableColumn];
    cell.stringValue = col < rowValues.count ? rowValues[col] : @"";
    cell.toolTip = cell.stringValue;
    return cell;
}

#pragma mark - Data Updates

/**
 * Parses a tab-separated, newline-delimited payload string into a 2-D array
 * of strings suitable for direct use by the table data source. Each row is
 * padded with empty strings to guarantee at least n columns.
 */
- (NSArray<NSArray<NSString *> *> *)parseRows:(NSString *)payload columns:(NSUInteger)n {
    NSMutableArray *result = [NSMutableArray array];
    for (NSString *line in [payload componentsSeparatedByCharactersInSet:
                             [NSCharacterSet newlineCharacterSet]]) {
        if (!line.length) continue;
        NSMutableArray *row = [[line componentsSeparatedByString:@"\t"] mutableCopy];
        while (row.count < n) [row addObject:@""];
        [result addObject:row];
    }
    return result;
}

/**
 * Replaces the frame table data with the parsed payload and reloads the table.
 * Must be called on the main thread.
 */
- (void)applyRowsPayload:(NSString *)payload {
    self.frameRows = [self parseRows:payload columns:4];
    [self.resultsTable reloadData];
    [self refreshEmptyState];
}

/**
 * Replaces the summary table data with the parsed payload and reloads the
 * table. Must be called on the main thread.
 */
- (void)applySummaryPayload:(NSString *)payload {
    self.summaryRows = [self parseRows:payload columns:6];
    [self.summaryTable reloadData];
    [self refreshEmptyState];
}

/**
 * Rebuilds the history popup items from the newline-separated payload and
 * selects the item at selectedIndex (-1 leaves the selection unchanged).
 * Sets updatingHistorySelection to suppress the historyChanged: callback
 * while modifying the popup programmatically. Must be called on the main thread.
 */
- (void)applyHistoryPayload:(NSString *)payload selectedIndex:(NSInteger)selectedIndex {
    NSMutableArray<NSString *> *items = [NSMutableArray array];
    for (NSString *s in [payload componentsSeparatedByCharactersInSet:
                          [NSCharacterSet newlineCharacterSet]]) {
        if (s.length) [items addObject:s];
    }
    self.historyItems = items;
    self.updatingHistorySelection = YES;
    [self.historyPopup removeAllItems];
    if (items.count > 0) {
        [self.historyPopup addItemsWithTitles:items];
        if (selectedIndex >= 0 && selectedIndex < (NSInteger)items.count) {
            [self.historyPopup selectItemAtIndex:selectedIndex];
        }
    } else {
        [self.historyPopup addItemWithTitle:@"No frames yet"];
    }
    self.updatingHistorySelection = NO;
    [self refreshHistoryControls];
}

/**
 * Shows or hides the "empty" placeholder labels based on whether the
 * corresponding table has any rows. Must be called on the main thread.
 */
- (void)refreshEmptyState {
    self.emptyLabel.hidden = (self.frameRows.count != 0);
    self.summaryEmptyLabel.hidden = (self.summaryRows.count != 0);
}

/**
 * Enables or disables the Prev/Next buttons and the history popup depending on
 * whether any frames are available and which item is currently selected.
 * Must be called on the main thread.
 */
- (void)refreshHistoryControls {
    BOOL has = (self.historyItems.count > 0);
    self.historyPopup.enabled = has;
    NSInteger sel = self.historyPopup.indexOfSelectedItem;
    self.previousFrameButton.enabled = has && sel > 0;
    self.nextFrameButton.enabled = has && sel >= 0 && sel + 1 < (NSInteger)self.historyItems.count;
}

#pragma mark - Helpers

/**
 * Creates a standard read-only, non-editable NSTextField label pre-filled with
 * value, sized and positioned by frame.
 */
- (NSTextField *)makeLabel:(NSString *)value frame:(NSRect)frame {
    NSTextField *lbl = [[NSTextField alloc] initWithFrame:frame];
    lbl.stringValue = value;
    lbl.bezeled = NO;
    lbl.drawsBackground = NO;
    lbl.editable = NO;
    lbl.selectable = NO;
    return lbl;
}

/**
 * Creates a lightweight section-header view: a semibold 11 pt secondary-colour
 * label above a 1 pt NSBox separator line, 22 pt tall.
 */
- (NSView *)makeSectionHeader:(NSString *)title width:(CGFloat)width {
    NSView *c = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, width, 22)];

    NSTextField *lbl = [self makeLabel:title frame:NSMakeRect(10, 4, width - 20, 15)];
    lbl.font = [NSFont systemFontOfSize:11 weight:NSFontWeightSemibold];
    lbl.textColor = [NSColor secondaryLabelColor];
    lbl.autoresizingMask = NSViewWidthSizable;
    [c addSubview:lbl];

    NSBox *sep = [[NSBox alloc] initWithFrame:NSMakeRect(0, 0, width, 1)];
    sep.boxType = NSBoxSeparator;
    sep.autoresizingMask = NSViewWidthSizable | NSViewMaxYMargin;
    [c addSubview:sep];

    return c;
}

/**
 * Creates an NSTableColumn with the given identifier, title, preferred width,
 * and minimum width. Resizing is user-controlled only (no auto-resizing by
 * default; the Command column overrides this after construction).
 */
- (NSTableColumn *)columnWithID:(NSString *)identifier
                          title:(NSString *)title
                          width:(CGFloat)width
                       minWidth:(CGFloat)minWidth {
    NSTableColumn *col = [[NSTableColumn alloc] initWithIdentifier:identifier];
    col.title = title;
    col.width = width;
    col.minWidth = minWidth;
    col.resizingMask = NSTableColumnUserResizingMask;
    return col;
}

@end

#pragma mark - C interface

/** Singleton delegate; set once in RunApp() and never changed. */
static MonitorAppDelegate *delegate;

/**
 * SetAppVersion stores version in gAppVersion. Must be called before RunApp()
 * so the string is available when applicationDidFinishLaunching: sets the
 * window title.
 */
void SetAppVersion(const char *version) {
    if (version) {
        gAppVersion = [NSString stringWithUTF8String:version];
    }
}

/**
 * RunApp creates the shared NSApplication, installs MonitorAppDelegate, and
 * starts the Cocoa event loop. Never returns.
 */
void RunApp(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        delegate = [[MonitorAppDelegate alloc] init];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        [NSApp setDelegate:delegate];
        [NSApp run];
    }
}

/**
 * UpdateResults is called from Go (ui_bridge.go) to push a complete UI
 * refresh. It converts the C strings to NSString and dispatches the actual
 * table/popup/status updates asynchronously onto the main queue.
 */
void UpdateResults(const char *status, const char *tableText,
                   const char *summaryText, const char *historyText,
                   int selectedIndex) {
    NSString *statusStr  = [NSString stringWithUTF8String:status      ?: ""];
    NSString *tableStr   = [NSString stringWithUTF8String:tableText   ?: ""];
    NSString *summaryStr = [NSString stringWithUTF8String:summaryText ?: ""];
    NSString *historyStr = [NSString stringWithUTF8String:historyText ?: ""];
    dispatch_async(dispatch_get_main_queue(), ^{
        delegate.statusLabel.stringValue = statusStr;
        [delegate applyRowsPayload:tableStr];
        [delegate applySummaryPayload:summaryStr];
        [delegate applyHistoryPayload:historyStr selectedIndex:selectedIndex];
    });
}

/**
 * ShowErrorMessage is called from Go (ui_bridge.go) to display an error in the
 * status bar and clear both tables. Dispatches to the main queue.
 */
void ShowErrorMessage(const char *message) {
    NSString *text = [NSString stringWithUTF8String:message ?: "Unknown error"];
    dispatch_async(dispatch_get_main_queue(), ^{
        delegate.statusLabel.stringValue = text;
        [delegate applyRowsPayload:@""];
        [delegate applySummaryPayload:@""];
        [delegate applyHistoryPayload:@"" selectedIndex:-1];
    });
}
