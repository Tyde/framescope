#!/usr/bin/env python3
import time
import psutil
import sys
import curses
import os
from datetime import timedelta

def usage():
    print("Usage: {} <measurement_interval_in_seconds>".format(sys.argv[0]))
    sys.exit(1)

if len(sys.argv) != 2:
    usage()

try:
    total_interval = float(sys.argv[1])
except ValueError:
    usage()

def snapshot():
    """
    Takes a snapshot of all running processes.
    Returns a dictionary mapping PID to a tuple: (total_cpu_seconds, command).
    """
    snap = {}
    for proc in psutil.process_iter(['pid', 'cpu_times', 'cmdline', 'name']):
        try:
            cpu_times = proc.info.get('cpu_times')
            if cpu_times is None:
                continue
            total_cpu = cpu_times.user + cpu_times.system
            cmdline = proc.info.get('cmdline')
            if cmdline:
                command = " ".join(cmdline)
            else:
                command = proc.info.get('name', '')
            snap[proc.info['pid']] = (total_cpu, command)
        except (psutil.NoSuchProcess, psutil.AccessDenied, psutil.ZombieProcess):
            continue
    return snap

def format_time(seconds):
    """Format seconds as HH:MM:SS."""
    return str(timedelta(seconds=int(seconds)))

def compute_live_results(initial_snap, current_snap):
    """
    For every process in the initial snapshot still present in current_snap,
    compute the CPU time difference.
    Returns a list of tuples: (pid, diff, command).
    """
    results = []
    for pid, (cpu1, cmd) in initial_snap.items():
        if pid in current_snap:
            cpu2, _ = current_snap[pid]
            diff = cpu2 - cpu1
            results.append((pid, diff, cmd))
    results.sort(key=lambda x: x[1], reverse=True)
    return results

def launch_tui(initial_snap, measurement_interval):
    """
    Launch a curses-based TUI with reduced flickering.
    Live updates occur until the measurement interval expires, then the final
    results are frozen for review.
    Controls:
      - Up/Down: Scroll vertically.
      - Left/Right: Scroll horizontally in the COMMAND column.
      - 'h': Toggle hiding processes with CPU time less than 1 sec.
      - 'p': Toggle between full command and just basename.
      - 'q': Quit.
    """
    # Default flags.
    hide_small = True   # Hide processes with CPU diff < 1 second.
    hide_paths = True

    def get_filtered_results(results, hide_small):
        return [r for r in results if r[1] >= 1] if hide_small else results

    def get_display_command(cmd, hide_paths):
        return os.path.basename(cmd) if hide_paths else cmd

    def format_command(cmd, field_width, horiz_offset):
        if len(cmd) <= field_width:
            return cmd.rjust(field_width)
        else:
            start = max(0, len(cmd) - field_width - horiz_offset)
            end = len(cmd) - horiz_offset
            segment = cmd[start:end]
            return segment.rjust(field_width)

    def draw_table(stdscr, results, vert_offset, horiz_offset, hide_small, hide_paths, remaining_time):
        # Instead of clearing the entire stdscr each time,
        # we use a dedicated subwindow for the table.
        height, width = stdscr.getmaxyx()

        # Create a subwindow inside the border.
        table_win = stdscr.derwin(height - 2, width - 2, 1, 1)
        table_win.erase()

        # Draw header in table_win.
        pid_width = 8
        raw_width = 8
        time_width = 18
        cmd_width = max(20, width - pid_width - raw_width - time_width - 6)  # extra space for borders

        header = f"{'PID':<{pid_width}} {'Raw(s)':<{raw_width}} {'COMMAND':>{cmd_width}} {'CPU Time (HH:MM:SS)':<{time_width}}"
        table_win.attron(curses.color_pair(1))
        table_win.addstr(0, 0, header)
        table_win.attroff(curses.color_pair(1))
        table_win.hline(1, 0, "-", width - 4)

        filtered_results = get_filtered_results(results, hide_small)
        max_rows = (height - 6)  # available rows in table_win
        
        for idx in range(max_rows):
            row_index = vert_offset + idx
            if row_index >= len(filtered_results):
                break
            pid, diff, cmd = filtered_results[row_index]
            cpu_time_str = format_time(diff)
            disp_cmd = get_display_command(cmd, hide_paths)
            disp_cmd_formatted = format_command(disp_cmd, cmd_width, horiz_offset)
            line = f"{pid:<{pid_width}} {diff:<{raw_width}.0f} {disp_cmd_formatted} {cpu_time_str:<{time_width}}"
            table_win.addstr(idx + 2, 0, line)
        
        # Draw status and footer on stdscr (outside the table window).
        status = f"Remaining time: {int(remaining_time)} sec" if remaining_time > 0 else "FINAL MEASUREMENT REACHED."
        footer = ("Arrows: Up/Down vertical, Left/Right horizontal | "
                  f"'h' toggle hide <1 sec (currently {'ON' if hide_small else 'OFF'}), "
                  f"'p' toggle hide paths (currently {'ON' if hide_paths else 'OFF'}) | "
                  "q to quit")
        stdscr.attron(curses.color_pair(2))
        stdscr.addstr(height - 2, 1, status[:width-2])
        stdscr.addstr(height - 1, 1, footer[:width-2])
        stdscr.attroff(curses.color_pair(2))

        # Instead of refreshing stdscr completely, refresh only table_win and then stdscr.
        table_win.noutrefresh()
        stdscr.noutrefresh()
        curses.doupdate()

    def live_mode(stdscr, initial_snap, measurement_interval):
        nonlocal hide_small, hide_paths
        curses.curs_set(0)
        stdscr.keypad(True)
        stdscr.nodelay(True)
        vert_offset = 0
        horiz_offset = 0

        start_time = time.time()
        final_time = start_time + measurement_interval

        results = compute_live_results(initial_snap, snapshot())
        while True:
            now = time.time()
            remaining = final_time - now
            if remaining > 0:
                results = compute_live_results(initial_snap, snapshot())
            else:
                break

            draw_table(stdscr, results, vert_offset, horiz_offset, hide_small, hide_paths, remaining)
            
            try:
                key = stdscr.getch()
            except curses.error:
                key = -1
            
            if key != -1:
                if key == curses.KEY_DOWN:
                    if vert_offset < len(get_filtered_results(results, hide_small)) - 1:
                        vert_offset += 1
                elif key == curses.KEY_UP:
                    if vert_offset > 0:
                        vert_offset -= 1
                elif key == curses.KEY_LEFT:
                    horiz_offset += 1
                elif key == curses.KEY_RIGHT:
                    if horiz_offset > 0:
                        horiz_offset -= 1
                elif key in (ord('h'), ord('H')):
                    hide_small = not hide_small
                    vert_offset = 0
                elif key in (ord('p'), ord('P')):
                    hide_paths = not hide_paths
                    horiz_offset = 0
                elif key in (ord('q'), ord('Q')):
                    return results
            time.sleep(0.5)
        return results

    def review_mode(stdscr, results):
        nonlocal hide_small, hide_paths
        curses.curs_set(0)
        stdscr.keypad(True)
        stdscr.nodelay(False)
        vert_offset = 0
        horiz_offset = 0

        while True:
            draw_table(stdscr, results, vert_offset, horiz_offset, hide_small, hide_paths, 0)
            key = stdscr.getch()
            if key == curses.KEY_DOWN:
                if vert_offset < len(get_filtered_results(results, hide_small)) - 1:
                    vert_offset += 1
            elif key == curses.KEY_UP:
                if vert_offset > 0:
                    vert_offset -= 1
            elif key == curses.KEY_LEFT:
                horiz_offset += 1
            elif key == curses.KEY_RIGHT:
                if horiz_offset > 0:
                    horiz_offset -= 1
            elif key in (ord('h'), ord('H')):
                hide_small = not hide_small
                vert_offset = 0
            elif key in (ord('p'), ord('P')):
                hide_paths = not hide_paths
                horiz_offset = 0
            elif key in (ord('q'), ord('Q')):
                break

    def main(stdscr):
        # Initialize colors.
        curses.start_color()
        curses.init_pair(1, curses.COLOR_BLACK, curses.COLOR_CYAN)   # Header
        curses.init_pair(2, curses.COLOR_YELLOW, curses.COLOR_BLACK)   # Footer/Status
        curses.init_pair(3, curses.COLOR_WHITE, curses.COLOR_BLUE)     # Title

        start_time = time.time()
        final_time = start_time + measurement_interval
        final_results = live_mode(stdscr, initial_snap, measurement_interval)
        review_mode(stdscr, final_results)

    curses.wrapper(main)

def main():
    print("Taking initial snapshot...")
    initial_snap = snapshot()
    print("Initial snapshot taken.")
    print(f"Measurement will run for {total_interval} seconds. Launching TUI...")
    launch_tui(initial_snap, measurement_interval=total_interval)

if __name__ == '__main__':
    main()
