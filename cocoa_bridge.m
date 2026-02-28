#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include "cocoa_bridge.h"

@interface MonitorAppDelegate : NSObject <NSApplicationDelegate>
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSTextField *frameField;
@property(nonatomic, strong) NSButton *startButton;
@property(nonatomic, strong) NSButton *stopButton;
@property(nonatomic, strong) NSButton *hideSmallButton;
@property(nonatomic, strong) NSButton *hidePathsButton;
@property(nonatomic, strong) NSTextField *statusLabel;
@property(nonatomic, strong) NSTextView *resultsView;
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

    NSView *content = self.window.contentView;

    NSTextField *title = [self label:@"Continuous CPU sampling in rolling frames" frame:NSMakeRect(20, 630, 420, 24)];
    title.font = [NSFont boldSystemFontOfSize:18];
    [content addSubview:title];

    NSTextField *frameLabel = [self label:@"Frame seconds" frame:NSMakeRect(20, 592, 110, 24)];
    [content addSubview:frameLabel];

    self.frameField = [[NSTextField alloc] initWithFrame:NSMakeRect(132, 588, 90, 28)];
    self.frameField.stringValue = @"5";
    [content addSubview:self.frameField];

    self.startButton = [self button:@"Start" frame:NSMakeRect(236, 586, 92, 32) action:@selector(startPressed:)];
    [content addSubview:self.startButton];

    self.stopButton = [self button:@"Stop" frame:NSMakeRect(338, 586, 92, 32) action:@selector(stopPressed:)];
    [content addSubview:self.stopButton];

    self.hideSmallButton = [self checkbox:@"Hide rows below 1s" frame:NSMakeRect(450, 590, 170, 24) action:@selector(hideSmallToggled:)];
    self.hideSmallButton.state = NSControlStateValueOn;
    [content addSubview:self.hideSmallButton];

    self.hidePathsButton = [self checkbox:@"Show basename only" frame:NSMakeRect(630, 590, 160, 24) action:@selector(hidePathsToggled:)];
    [content addSubview:self.hidePathsButton];

    self.statusLabel = [self label:@"Idle. Set a frame length and press Start." frame:NSMakeRect(20, 548, 920, 24)];
    self.statusLabel.font = [NSFont systemFontOfSize:13 weight:NSFontWeightMedium];
    [content addSubview:self.statusLabel];

    NSScrollView *scrollView = [[NSScrollView alloc] initWithFrame:NSMakeRect(20, 20, 940, 510)];
    scrollView.hasVerticalScroller = YES;
    scrollView.hasHorizontalScroller = YES;
    scrollView.autohidesScrollers = YES;
    scrollView.borderType = NSBezelBorder;

    self.resultsView = [[NSTextView alloc] initWithFrame:scrollView.bounds];
    self.resultsView.editable = NO;
    self.resultsView.selectable = YES;
    self.resultsView.richText = NO;
    self.resultsView.automaticQuoteSubstitutionEnabled = NO;
    self.resultsView.automaticDashSubstitutionEnabled = NO;
    self.resultsView.font = [NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightRegular];
    self.resultsView.string = @" PID      Raw(s)   CPU Time     Command\n\n Press Start to begin.";

    scrollView.documentView = self.resultsView;
    [content addSubview:scrollView];

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

- (NSTextField *)label:(NSString *)value frame:(NSRect)frame {
    NSTextField *label = [[NSTextField alloc] initWithFrame:frame];
    label.stringValue = value;
    label.bezeled = NO;
    label.drawsBackground = NO;
    label.editable = NO;
    label.selectable = NO;
    return label;
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

void UpdateResults(const char *status, const char *tableText) {
    NSString *statusString = [NSString stringWithUTF8String:status ?: ""];
    NSString *tableString = [NSString stringWithUTF8String:tableText ?: ""];
    dispatch_async(dispatch_get_main_queue(), ^{
        delegate.statusLabel.stringValue = statusString;
        delegate.resultsView.string = tableString;
    });
}

void ShowErrorMessage(const char *message) {
    NSString *text = [NSString stringWithUTF8String:message ?: "Unknown error"];
    dispatch_async(dispatch_get_main_queue(), ^{
        delegate.statusLabel.stringValue = text;
    });
}
