#!/bin/bash
# monitor_all_cpu_pretty.sh
# Usage: ./monitor_all_cpu_pretty.sh <interval_in_seconds>
# This script snapshots all processes’ cumulative CPU time,
# waits for the defined interval, then calculates and displays
# how much CPU time each process used over that period.
# It pretty prints the output in nicely aligned columns.

if [ $# -ne 1 ]; then
    echo "Usage: $0 <interval_in_seconds>"
    exit 1
fi

INTERVAL=$1

# Create temporary files to hold snapshots.
SNAP1=$(mktemp)
SNAP2=$(mktemp)

# Take initial snapshot.
ps -ax -o pid,cputime,comm > "$SNAP1"
echo "Initial snapshot taken."

echo "Waiting for $INTERVAL seconds..."
sleep "$INTERVAL"

# Take final snapshot.
ps -ax -o pid,cputime,comm > "$SNAP2"
echo "Final snapshot taken."

# Process the two snapshots.
# This AWK script:
#  - Reads the first snapshot (skipping the header) and stores the initial CPU time (in seconds)
#    and command for each PID.
#  - Then, for the second snapshot (skipping its header), it computes the difference for each PID
#    that exists in both snapshots.
#  - It prints: raw seconds difference, PID, command, and formatted time (HH:MM:SS).
awk '
function time_to_seconds(time_str) {
    # Accepts "HH:MM:SS" or "D-HH:MM:SS"
    split(time_str, parts, "-");
    if (length(parts) == 2) {
        days = parts[1];
        time_part = parts[2];
    } else {
        days = 0;
        time_part = parts[1];
    }
    split(time_part, tparts, ":");
    hh = tparts[1]; mm = tparts[2]; ss = tparts[3];
    return days * 86400 + hh * 3600 + mm * 60 + ss;
}
BEGIN {
    OFS="\t";
}
NR==FNR && FNR > 1 {
    # For the first snapshot, store the initial CPU time and command for each PID.
    init[$1] = time_to_seconds($2);
    cmd[$1] = $3;
    next;
}
FNR==1 { next }  # Skip header of the second snapshot.
{
    pid = $1;
    final = time_to_seconds($2);
    if (pid in init) {
        diff = final - init[pid];
        if (diff > 0) {
            # Print: raw diff, PID, command, and formatted time HH:MM:SS.
            printf "%d\t%s\t%s\t%02d:%02d:%02d\n", diff, pid, cmd[pid], diff/3600, (diff % 3600)/60, diff % 60;
        }
    }
}
' "$SNAP1" "$SNAP2" > /tmp/cpu_diff_raw.txt

# Sort the output in descending order by the raw CPU time difference (first field).
sort -nr /tmp/cpu_diff_raw.txt > /tmp/cpu_diff_sorted.txt

# Print header and then the results with nicely aligned columns.
{
  printf "%-8s %-8s %-40s %s\n" "PID" "Raw(s)" "COMMAND" "CPU Time Used (HH:MM:SS)";
  printf "%-8s %-8s %-40s %s\n" "----" "------" "-------" "----------------------";
  # Use AWK to format each line, then pipe through column for even spacing.
  awk -F"\t" '{ printf "%-8s %-8s %-40s %s\n", $2, $1, $3, $4 }' /tmp/cpu_diff_sorted.txt
} | column -t

# Clean up temporary files.
rm "$SNAP1" "$SNAP2" /tmp/cpu_diff_raw.txt /tmp/cpu_diff_sorted.txt

