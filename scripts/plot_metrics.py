#!/usr/bin/env python3
"""
scripts/plot_metrics.py — Visualization for the Checkers AI project report.

Generates:
  1. PST heatmaps (2×2 grid): MenOpening, MenEndgame, KingsOpening, KingsEndgame
  2. Training learning curve stacked area chart (Win / Draw / Loss %)

Usage:
  python scripts/plot_metrics.py                          # uses default paths
  python scripts/plot_metrics.py --weights path/to/pst_weights.json \
                                 --log    path/to/training_log.csv  \
                                 --out    reports/
"""

import argparse
import json
import os

import matplotlib.pyplot as plt
import matplotlib.colors as mcolors
import matplotlib.patches as mpatches
import numpy as np
import pandas as pd
import seaborn as sns

# ── Board geometry ────────────────────────────────────────────────────────────

def sq_to_grid(sq: int) -> tuple[int, int]:
    """Map internal square (0-31) → (visual_row, col) on an 8×8 board.

    Internal layout: sq // 4 = row from bottom (0 = Red home, 7 = Black home).
    Display: row 0 at the top (Black home), row 7 at the bottom (Red home).
    Even internal rows use even board columns (0,2,4,6).
    Odd  internal rows use odd  board columns (1,3,5,7).
    """
    internal_row = sq // 4      # 0 = Red home (bottom)
    p            = sq % 4
    visual_row   = 7 - internal_row   # flip: 0 = top = Black home
    col          = 2 * p if internal_row % 2 == 0 else 2 * p + 1
    return visual_row, col


def weights_to_grid(weights_32: list[float]) -> np.ndarray:
    """Convert a 32-element weight list into an 8×8 grid (NaN on light squares)."""
    grid = np.full((8, 8), np.nan)
    for sq, val in enumerate(weights_32):
        r, c = sq_to_grid(sq)
        grid[r, c] = val
    return grid


# ── Part 1: PST Heatmaps ──────────────────────────────────────────────────────

def plot_pst_heatmaps(weights_path: str, out_dir: str) -> None:
    """Read pst_weights.json and save a 2×2 heatmap figure."""
    with open(weights_path) as f:
        w = json.load(f)

    tables = {
        "Men — Opening":   w["men_opening"],
        "Men — Endgame":   w["men_endgame"],
        "Kings — Opening": w["kings_opening"],
        "Kings — Endgame": w["kings_endgame"],
    }

    # Shared colour scale centred on zero
    all_vals = [v for arr in tables.values() for v in arr if v != 0]
    vmax = max(abs(v) for v in all_vals) if all_vals else 1.0
    cmap = "RdYlGn"              # Red = negative, Yellow = neutral, Green = positive

    fig, axes = plt.subplots(2, 2, figsize=(12, 10))
    fig.suptitle("Tapered Piece-Square Tables — Learnt Weights", fontsize=16, y=1.01)

    for ax, (title, weights) in zip(axes.flat, tables.items()):
        grid = weights_to_grid(weights)

        # Checkerboard mask overlay
        mask = np.zeros((8, 8), dtype=bool)
        for r in range(8):
            for c in range(8):
                # Light squares are unplayable — mark as masked
                if (r + c) % 2 == 0:
                    mask[r, c] = True

        sns.heatmap(
            grid,
            ax=ax,
            mask=mask,
            annot=True,
            fmt=".1f",
            cmap=cmap,
            vmin=-vmax,
            vmax=vmax,
            linewidths=0.5,
            linecolor="grey",
            cbar_kws={"shrink": 0.7},
            square=True,
            annot_kws={"size": 7},
        )

        # Grey out light squares
        light_mask = ~mask
        for r in range(8):
            for c in range(8):
                if (r + c) % 2 == 0:
                    ax.add_patch(plt.Rectangle((c, r), 1, 1, color="#D0D0D0", zorder=2))

        ax.set_title(title, fontsize=12, pad=6)
        ax.set_xticks(np.arange(8) + 0.5)
        ax.set_xticklabels(list("ABCDEFGH"), fontsize=8)
        ax.set_yticks(np.arange(8) + 0.5)
        ax.set_yticklabels(["8","7","6","5","4","3","2","1"], fontsize=8)
        ax.tick_params(length=0)

    plt.tight_layout()
    out_path = os.path.join(out_dir, "pst_heatmaps.png")
    fig.savefig(out_path, dpi=150, bbox_inches="tight")
    print(f"Saved: {out_path}")
    plt.close(fig)


# ── Part 2: Training Learning Curve ───────────────────────────────────────────

def plot_learning_curve(log_path: str, out_dir: str) -> None:
    """Read training_log.csv and plot a stacked area chart of W/D/L percentages."""
    df = pd.read_csv(log_path)
    # Expected columns: game, wins, draws, losses
    df.columns = [c.lower().strip() for c in df.columns]

    total = df["wins"] + df["draws"] + df["losses"]
    df["win_pct"]  = df["wins"]   / total * 100
    df["draw_pct"] = df["draws"]  / total * 100
    df["loss_pct"] = df["losses"] / total * 100

    # Smooth with rolling average for cleaner area chart
    window = max(1, len(df) // 20)
    win_s  = df["win_pct"].rolling(window, min_periods=1).mean()
    draw_s = df["draw_pct"].rolling(window, min_periods=1).mean()
    loss_s = df["loss_pct"].rolling(window, min_periods=1).mean()

    fig, ax = plt.subplots(figsize=(12, 5))
    games = df["game"]

    ax.stackplot(
        games,
        win_s, draw_s, loss_s,
        labels=["Win (Red)", "Draw", "Loss (Red)"],
        colors=["#2ecc71", "#f39c12", "#e74c3c"],
        alpha=0.85,
    )

    ax.set_xlabel("Self-Play Game #", fontsize=12)
    ax.set_ylabel("Outcome %", fontsize=12)
    ax.set_title("TD-Leaf(λ) Training Progress — Win / Draw / Loss Over Time", fontsize=14)
    ax.set_xlim(games.iloc[0], games.iloc[-1])
    ax.set_ylim(0, 100)
    ax.legend(loc="upper right", fontsize=10)
    ax.yaxis.set_major_formatter(plt.FuncFormatter(lambda v, _: f"{v:.0f}%"))
    ax.grid(axis="y", linestyle="--", alpha=0.4)
    sns.despine(ax=ax)

    out_path = os.path.join(out_dir, "training_curve.png")
    fig.savefig(out_path, dpi=150, bbox_inches="tight")
    print(f"Saved: {out_path}")
    plt.close(fig)

# ── Part 3: New Visualizations ────────────────────────────────────────────────

def plot_sa_cooling_curve(log_path: str, out_dir: str = "reports") -> None:
    """Plot SA cooling curve: Energy and best energy vs Temp over iterations."""
    df = pd.read_csv(log_path)
    df.columns = [c.lower().strip() for c in df.columns]

    fig, ax1 = plt.subplots(figsize=(10, 6))

    ax1.plot(df['iteration'], df['energy'], color='#3498db', label='Energy', alpha=0.7)
    ax1.plot(df['iteration'], df['best_energy'], color='#2980b9', label='Best Energy', linewidth=2)
    ax1.set_xlabel('Iteration', fontsize=12)
    ax1.set_ylabel('Energy', color='#2980b9', fontsize=12)
    ax1.tick_params(axis='y', labelcolor='#2980b9')

    ax2 = ax1.twinx()
    ax2.plot(df['iteration'], df['temp'], color='#e74c3c', linestyle='--', label='Temperature', linewidth=2)
    ax2.set_ylabel('Temperature', color='#e74c3c', fontsize=12)
    ax2.tick_params(axis='y', labelcolor='#e74c3c')

    fig.suptitle('Simulated Annealing: Energy and Cooling Schedule', fontsize=14)
    
    # Combine legends
    lines_1, labels_1 = ax1.get_legend_handles_labels()
    lines_2, labels_2 = ax2.get_legend_handles_labels()
    ax2.legend(lines_1 + lines_2, labels_1 + labels_2, loc='upper right')

    plt.tight_layout()
    out_path = os.path.join(out_dir, "sa_cooling_curve.png")
    fig.savefig(out_path, dpi=150, bbox_inches="tight")
    print(f"Saved: {out_path}")
    plt.close(fig)

def plot_search_efficiency(csv_path: str, out_dir: str = "reports") -> None:
    """Plot search efficiency comparing Negamax and PVS."""
    df = pd.read_csv(csv_path)
    
    fig, ax1 = plt.subplots(figsize=(8, 6))
    
    bar_width = 0.4
    index = np.arange(len(df['Algorithm']))
    
    # Left Axis: Nodes Expanded (Bars)
    bars1 = ax1.bar(index, df['NodesExpanded'], bar_width, color='#3498db', label='Nodes Expanded')
    ax1.set_xlabel('Algorithm', fontsize=12)
    ax1.set_ylabel('Nodes Expanded', color='#3498db', fontsize=12)
    ax1.set_xticks(index)
    ax1.set_xticklabels(df['Algorithm'], fontsize=11)
    ax1.tick_params(axis='y', labelcolor='#3498db')
    
    # Right Axis: Max Depth (Line/Points)
    ax2 = ax1.twinx()
    line1 = ax2.plot(index, df['MaxDepth'], color='#e74c3c', marker='o', markersize=8, linestyle='-', linewidth=2, label='Max Depth')
    ax2.set_ylabel('Max Depth Reached', color='#e74c3c', fontsize=12)
    ax2.tick_params(axis='y', labelcolor='#e74c3c')
    
    # Set y-axis limits to ensure line plots slightly above bars visually
    ax2.set_ylim(0, max(df['MaxDepth']) * 1.2)
    
    fig.suptitle('Search Efficiency Comparison: Negamax vs PVS', fontsize=14)
    
    lines_1, labels_1 = ax1.get_legend_handles_labels()
    lines_2, labels_2 = ax2.get_legend_handles_labels()
    ax2.legend(lines_1 + lines_2, labels_1 + labels_2, loc='upper left')

    plt.tight_layout()
    out_path = os.path.join(out_dir, "search_efficiency.png")
    fig.savefig(out_path, dpi=150, bbox_inches="tight")
    print(f"Saved: {out_path}")
    plt.close(fig)

def plot_tournament_matrix(csv_path: str, out_dir: str = "reports") -> None:
    """Plot the tournament results as a heatmap showing P1 Win Rate."""
    df = pd.read_csv(csv_path)
    
    total_games = df['P1_Wins'] + df['P2_Wins'] + df['Draws']
    df['WinRate'] = (df['P1_Wins'] + 0.5 * df['Draws']) / total_games
    
    matrix = df.pivot(index="Player1", columns="Player2", values="WinRate")
    
    fig, ax = plt.subplots(figsize=(8, 6))
    
    sns.heatmap(
        matrix, 
        annot=True, 
        fmt=".0%", 
        cmap="viridis", 
        cbar_kws={'label': 'Win Rate (inc. 0.5 for draws)'},
        linewidths=.5,
        ax=ax
    )
    
    ax.set_title("Tournament Results (Row vs Column Win Rate)", fontsize=14, pad=15)
    ax.set_ylabel("Player 1", fontsize=12)
    ax.set_xlabel("Player 2", fontsize=12)
    
    plt.tight_layout()
    out_path = os.path.join(out_dir, "tournament_matrix.png")
    fig.savefig(out_path, dpi=150, bbox_inches="tight")
    print(f"Saved: {out_path}")
    plt.close(fig)


def main():
    parser = argparse.ArgumentParser(description="Checkers AI metric visualizations")
    parser.add_argument("--weights", default="weights/pst_weights.json",
                        help="Path to pst_weights.json")
    parser.add_argument("--log", default="training_log.csv",
                        help="Path to training_log.csv produced by cmd/train")
    parser.add_argument("--out", default="reports",
                        help="Output directory for PNG images")
    args = parser.parse_args()

    os.makedirs(args.out, exist_ok=True)

    if os.path.exists(args.weights):
        plot_pst_heatmaps(args.weights, args.out)
    else:
        print(f"WARN: weights file not found at '{args.weights}' — skipping heatmaps.")

    if os.path.exists(args.log):
        plot_learning_curve(args.log, args.out)
    else:
        print(f"WARN: training log not found at '{args.log}' — skipping learning curve.")

    sa_log = "training_sa_log.csv"
    if os.path.exists(sa_log):
        plot_sa_cooling_curve(sa_log, args.out)

    efficiency_csv = "search_efficiency.csv"
    if os.path.exists(efficiency_csv):
        plot_search_efficiency(efficiency_csv, args.out)

    tournament_csv = "tournament_results.csv"
    if os.path.exists(tournament_csv):
        plot_tournament_matrix(tournament_csv, args.out)

if __name__ == "__main__":
    main()
