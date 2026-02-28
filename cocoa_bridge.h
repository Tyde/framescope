#ifndef MONITOR_CPU_COCOA_BRIDGE_H
#define MONITOR_CPU_COCOA_BRIDGE_H

void RunApp(void);
void UpdateResults(const char *status, const char *tableText, const char *summaryText, const char *historyText, int selectedIndex);
void ShowErrorMessage(const char *message);

void GoStartMonitoring(double frameSeconds);
void GoStopMonitoring(void);
void GoSetHideSmall(int enabled);
void GoSetHidePaths(int enabled);
void GoSelectFrame(int selectedIndex);
int GoInitialHideSmall(void);
int GoInitialHidePaths(void);
double GoInitialFrameSeconds(void);

#endif
