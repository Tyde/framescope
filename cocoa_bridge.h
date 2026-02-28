#ifndef MONITOR_CPU_COCOA_BRIDGE_H
#define MONITOR_CPU_COCOA_BRIDGE_H

void RunApp(void);
void UpdateResults(const char *status, const char *tableText);
void ShowErrorMessage(const char *message);

void GoStartMonitoring(double frameSeconds);
void GoStopMonitoring(void);
void GoSetHideSmall(int enabled);
void GoSetHidePaths(int enabled);

#endif
