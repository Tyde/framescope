#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include "cocoa_bridge.h"

@interface MonitorAppDelegate : NSObject <NSApplicationDelegate, NSTableViewDataSource, NSTableViewDelegate>
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSTextField *frameField;
@property(nonatomic, strong) NSButton *startButton;
@property(nonatomic, strong) NSButton *stopButton;
@property(nonatomic, strong) NSButton *hideSmallButton;
@property(nonatomic, strong) NSButton *hidePathsButton;
@property(nonatomic, strong) NSTextField *historyLabel;
@property(nonatomic, strong) NSPopUpButton *historyPopup;
@property(nonatomic, strong) NSButton *previousFrameButton;
@property(nonatomic, strong) NSButton *nextFrameButton;
@property(nonatomic, strong) NSTextField *statusLabel;
@property(nonatomic, strong) NSScrollView *tableScrollView;
@property(nonatomic, strong) NSTableView *resultsTable;
@property(nonatomic, strong) NSTextField *emptyLabel;
@property(nonatomic, copy) NSArray<NSArray<NSString *> *> *rows;
@property(nonatomic, copy) NSArray<NSString *> *historyItems;
@property(nonatomic, assign) BOOL updatingHistorySelection;
@end

@implementation MonitorAppDelegate

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    (void)notification;

    self.window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 980, 680)
                                              styleMask:(NSWindowStyleMaskTitled |
                                                         NSWindowStyleMaskClosable |
                                                         NSWindowStyleMaskMiniaturizable |
                                                         NSWindowStyleMaskResizable)
                                                backing:NSBackingStoreBuffered
                                                  defer:NO];
    [self.window setTitle:@"CPU Frame Monitor"];
    [self.window center];
    [self.window setMinSize:NSMakeSize(720, 420)];

    NSView *content = self.window.contentView;
    content.autoresizesSubviews = YES;

    NSTextField *title = [self label:@"Continuous CPU sampling in rolling frames" frame:NSMakeRect(20, 630, 420, 24)];
    title.font = [NSFont boldSystemFontOfSize:18];
    title.autoresizingMask = NSViewMinYMargin;
    [content addSubview:title];

    NSTextField *frameLabel = [self label:@"Frame seconds" frame:NSMakeRect(20, 592, 110, 24)];
    frameLabel.autoresizingMask = NSViewMinYMargin;
    [content addSubview:frameLabel];

    self.frameField = [[NSTextField alloc] initWithFrame:NSMakeRect(132, 588, 90, 28)];
    self.frameField.stringValue = @"5";
    self.frameField.autoresizingMask = NSViewMinYMargin;
    [content addSubview:self.frameField];

    self.startButton = [self button:@"Start" frame:NSMakeRect(236, 586, 92, 32) action:@selector(startPressed:)];
    self.startButton.autoresizingMask = NSViewMinYMargin;
    [content addSubview:self.startButton];

    self.stopButton = [self button:@"Stop" frame:NSMakeRect(338, 586, 92, 32) action:@selector(stopPressed:)];
    self.stopButton.autoresizingMask = NSViewMinYMargin;
    [content addSubview:self.stopButton];

    self.hideSmallButton = [self checkbox:@"Hide rows below 1s" frame:NSMakeRect(450, 590, 170, 24) action:@selector(hideSmallToggled:)];
    self.hideSmallButton.state = NSControlStateValueOn;
    self.hideSmallButton.autoresizingMask = NSViewMinYMargin;
    [content addSubview:self.hideSmallButton];

    self.hidePathsButton = [self checkbox:@"Show basename only" frame:NSMakeRect(630, 590, 160, 24) action:@selector(hidePathsToggled:)];
    self.hidePathsButton.autoresizingMask = NSViewMinYMargin;
    [content addSubview:self.hidePathsButton];

    self.historyLabel = [self label:@"View" frame:NSMakeRect(20, 552, 48, 24)];
    self.historyLabel.autoresizingMask = NSViewMinYMargin;
    [content addSubview:self.historyLabel];

    self.historyPopup = [[NSPopUpButton alloc] initWithFrame:NSMakeRect(68, 548, 250, 28) pullsDown:NO];
    self.historyPopup.target = self;
    self.historyPopup.action = @selector(historyChanged:);
    self.historyPopup.autoresizingMask = NSViewMinYMargin;
    self.historyPopup.enabled = NO;
    [content addSubview:self.historyPopup];

    self.previousFrameButton = [self button:@"Previous" frame:NSMakeRect(332, 546, 96, 32) action:@selector(previousFrame:)];
    self.previousFrameButton.autoresizingMask = NSViewMinYMargin;
    self.previousFrameButton.enabled = NO;
    [content addSubview:self.previousFrameButton];

    self.nextFrameButton = [self button:@"Next" frame:NSMakeRect(438, 546, 80, 32) action:@selector(nextFrame:)];
    self.nextFrameButton.autoresizingMask = NSViewMinYMargin;
    self.nextFrameButton.enabled = NO;
    [content addSubview:self.nextFrameButton];

    self.statusLabel = [self label:@"Idle. Set a frame length and press Start." frame:NSMakeRect(20, 516, 920, 24)];
    self.statusLabel.font = [NSFont systemFontOfSize:13 weight:NSFontWeightMedium];
    self.statusLabel.autoresizingMask = NSViewWidthSizable | NSViewMinYMargin;
    [content addSubview:self.statusLabel];

    self.tableScrollView = [[NSScrollView alloc] initWithFrame:NSMakeRect(20, 20, 940, 478)];
    self.tableScrollView.hasVerticalScroller = YES;
    self.tableScrollView.hasHorizontalScroller = YES;
    self.tableScrollView.autohidesScrollers = YES;
    self.tableScrollView.borderType = NSBezelBorder;
    self.tableScrollView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

    self.resultsTable = [[NSTableView alloc] initWithFrame:self.tableScrollView.bounds];
    self.resultsTable.usesAlternatingRowBackgroundColors = YES;
    self.resultsTable.allowsColumnResizing = YES;
    self.resultsTable.allowsTypeSelect = YES;
    self.resultsTable.rowSizeStyle = NSTableViewRowSizeStyleDefault;
    self.resultsTable.gridStyleMask = NSTableViewSolidVerticalGridLineMask;
    self.resultsTable.dataSource = self;
    self.resultsTable.delegate = self;
    self.resultsTable.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;

    [self.resultsTable addTableColumn:[self tableColumnWithID:@"pid" title:@"PID" width:90 minWidth:70]];
    [self.resultsTable addTableColumn:[self tableColumnWithID:@"raw" title:@"Raw(s)" width:90 minWidth:70]];
    [self.resultsTable addTableColumn:[self tableColumnWithID:@"cpu" title:@"CPU Time" width:120 minWidth:100]];
    NSTableColumn *commandColumn = [self tableColumnWithID:@"command" title:@"Command" width:620 minWidth:200];
    commandColumn.resizingMask = NSTableColumnAutoresizingMask | NSTableColumnUserResizingMask;
    [self.resultsTable addTableColumn:commandColumn];

    self.tableScrollView.documentView = self.resultsTable;
    [content addSubview:self.tableScrollView];

    self.emptyLabel = [self label:@"Press Start to begin." frame:NSMakeRect(32, 36, 320, 22)];
    self.emptyLabel.textColor = [NSColor secondaryLabelColor];
    self.emptyLabel.autoresizingMask = NSViewMaxXMargin | NSViewMaxYMargin;
    [content addSubview:self.emptyLabel];

    self.rows = @[];
    self.historyItems = @[];
    [self refreshEmptyState];
    [self refreshHistoryControls];

    [self.window makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
}

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)sender {
    (void)sender;
    GoStopMonitoring();
    return YES;
}

- (void)startPressed:(id)sender {
    (void)sender;
    double frameSeconds = self.frameField.doubleValue;
    GoStartMonitoring(frameSeconds);
}

- (void)stopPressed:(id)sender {
    (void)sender;
    GoStopMonitoring();
}

- (void)hideSmallToggled:(id)sender {
    (void)sender;
    GoSetHideSmall(self.hideSmallButton.state == NSControlStateValueOn ? 1 : 0);
}

- (void)hidePathsToggled:(id)sender {
    (void)sender;
    GoSetHidePaths(self.hidePathsButton.state == NSControlStateValueOn ? 1 : 0);
}

- (void)historyChanged:(id)sender {
    (void)sender;
    if (self.updatingHistorySelection) {
        return;
    }
    GoSelectFrame((int)self.historyPopup.indexOfSelectedItem);
}

- (void)previousFrame:(id)sender {
    (void)sender;
    NSInteger index = self.historyPopup.indexOfSelectedItem;
    if (index > 0) {
        [self.historyPopup selectItemAtIndex:index - 1];
        [self historyChanged:self.historyPopup];
    }
}

- (void)nextFrame:(id)sender {
    (void)sender;
    NSInteger index = self.historyPopup.indexOfSelectedItem;
    if (index >= 0 && index + 1 < self.historyPopup.numberOfItems) {
        [self.historyPopup selectItemAtIndex:index + 1];
        [self historyChanged:self.historyPopup];
    }
}

- (NSTextField *)label:(NSString *)value frame:(NSRect)frame {
    NSTextField *label = [[NSTextField alloc] initWithFrame:frame];
    label.stringValue = value;
    label.bezeled = NO;
    label.drawsBackground = NO;
    label.editable = NO;
    label.selectable = NO;
    return label;
}

- (NSTableColumn *)tableColumnWithID:(NSString *)identifier title:(NSString *)title width:(CGFloat)width minWidth:(CGFloat)minWidth {
    NSTableColumn *column = [[NSTableColumn alloc] initWithIdentifier:identifier];
    column.title = title;
    column.width = width;
    column.minWidth = minWidth;
    column.resizingMask = NSTableColumnUserResizingMask;
    return column;
}

- (NSInteger)numberOfRowsInTableView:(NSTableView *)tableView {
    (void)tableView;
    return self.rows.count;
}

- (nullable NSView *)tableView:(NSTableView *)tableView viewForTableColumn:(NSTableColumn *)tableColumn row:(NSInteger)row {
    (void)tableView;
    NSString *identifier = tableColumn.identifier;
    NSTextField *cell = [self.resultsTable makeViewWithIdentifier:identifier owner:self];
    if (cell == nil) {
        cell = [[NSTextField alloc] initWithFrame:NSZeroRect];
        cell.identifier = identifier;
        cell.bezeled = NO;
        cell.drawsBackground = NO;
        cell.editable = NO;
        cell.selectable = YES;
        cell.lineBreakMode = NSLineBreakByTruncatingMiddle;
        cell.font = [NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightRegular];
    }

    NSArray<NSString *> *rowValues = self.rows[(NSUInteger)row];
    NSUInteger columnIndex = [self.resultsTable.tableColumns indexOfObject:tableColumn];
    cell.stringValue = columnIndex < rowValues.count ? rowValues[columnIndex] : @"";
    cell.toolTip = cell.stringValue;
    return cell;
}

- (void)applyRowsPayload:(NSString *)payload {
    NSMutableArray<NSArray<NSString *> *> *parsedRows = [NSMutableArray array];
    NSArray<NSString *> *lines = [payload componentsSeparatedByCharactersInSet:[NSCharacterSet newlineCharacterSet]];
    for (NSString *line in lines) {
        if (line.length == 0) {
            continue;
        }
        NSArray<NSString *> *parts = [line componentsSeparatedByString:@"\t"];
        NSMutableArray<NSString *> *row = [NSMutableArray arrayWithArray:parts];
        while (row.count < 4) {
            [row addObject:@""];
        }
        [parsedRows addObject:row];
    }
    self.rows = parsedRows;
    [self.resultsTable reloadData];
    [self refreshEmptyState];
}

- (void)applyHistoryPayload:(NSString *)payload selectedIndex:(NSInteger)selectedIndex {
    NSArray<NSString *> *items = payload.length == 0 ? @[] : [payload componentsSeparatedByCharactersInSet:[NSCharacterSet newlineCharacterSet]];
    NSMutableArray<NSString *> *cleanItems = [NSMutableArray array];
    for (NSString *item in items) {
        if (item.length > 0) {
            [cleanItems addObject:item];
        }
    }

    self.historyItems = cleanItems;
    self.updatingHistorySelection = YES;
    [self.historyPopup removeAllItems];
    if (cleanItems.count > 0) {
        [self.historyPopup addItemsWithTitles:cleanItems];
    }
    if (selectedIndex >= 0 && selectedIndex < (NSInteger)cleanItems.count) {
        [self.historyPopup selectItemAtIndex:selectedIndex];
    }
    self.updatingHistorySelection = NO;
    [self refreshHistoryControls];
}

- (void)refreshEmptyState {
    self.emptyLabel.hidden = (self.rows.count != 0);
}

- (void)refreshHistoryControls {
    BOOL hasItems = (self.historyItems.count > 0);
    self.historyPopup.enabled = hasItems;
    NSInteger selectedIndex = self.historyPopup.indexOfSelectedItem;
    self.previousFrameButton.enabled = hasItems && selectedIndex > 0;
    self.nextFrameButton.enabled = hasItems && selectedIndex >= 0 && selectedIndex + 1 < self.historyItems.count;
}

- (NSButton *)button:(NSString *)title frame:(NSRect)frame action:(SEL)action {
    NSButton *button = [[NSButton alloc] initWithFrame:frame];
    button.title = title;
    button.bezelStyle = NSBezelStyleRounded;
    button.target = self;
    button.action = action;
    return button;
}

- (NSButton *)checkbox:(NSString *)title frame:(NSRect)frame action:(SEL)action {
    NSButton *button = [[NSButton alloc] initWithFrame:frame];
    button.title = title;
    button.buttonType = NSButtonTypeSwitch;
    button.target = self;
    button.action = action;
    return button;
}

@end

static MonitorAppDelegate *delegate;

void RunApp(void) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        delegate = [[MonitorAppDelegate alloc] init];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
        [NSApp setDelegate:delegate];
        [NSApp run];
    }
}

void UpdateResults(const char *status, const char *tableText, const char *historyText, int selectedIndex) {
    NSString *statusString = [NSString stringWithUTF8String:status ?: ""];
    NSString *tableString = [NSString stringWithUTF8String:tableText ?: ""];
    NSString *historyString = [NSString stringWithUTF8String:historyText ?: ""];
    dispatch_async(dispatch_get_main_queue(), ^{
        delegate.statusLabel.stringValue = statusString;
        [delegate applyRowsPayload:tableString];
        [delegate applyHistoryPayload:historyString selectedIndex:selectedIndex];
    });
}

void ShowErrorMessage(const char *message) {
    NSString *text = [NSString stringWithUTF8String:message ?: "Unknown error"];
    dispatch_async(dispatch_get_main_queue(), ^{
        delegate.statusLabel.stringValue = text;
        [delegate applyRowsPayload:@""];
        [delegate applyHistoryPayload:@"" selectedIndex:-1];
    });
}
