#!/usr/bin/env python3
"""
build_whitepaper_pdf.py
Compiles docs/THE_AUTONOMOUS_AGENT_RUNTIME_WHITEPAPER.md into an executive-grade,
typeset PDF document suitable for LinkedIn document carousel posts, Slack sharing,
and formal distribution.
"""

import os
import re
import html
import shutil
import subprocess
from markdown_it import MarkdownIt

def build_pdf():
    repo_root = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    md_path = os.path.join(repo_root, "docs", "THE_AUTONOMOUS_AGENT_RUNTIME_WHITEPAPER.md")
    
    with open(md_path, "r", encoding="utf-8") as f:
        md_text = f.read()

    # Strip redundant top title and metadata block from markdown since it's in the cover banner
    if "---" in md_text:
        parts = md_text.split("---", 1)
        md_text = parts[1].strip()

    # Pre-process GitHub alerts: > [!NOTE]
    alert_pattern = r'> \[!NOTE\]\s*\n((?:> .*\n?)+)'
    def replace_alert(match):
        lines = match.group(1).splitlines()
        cleaned_lines = [re.sub(r'^>\s?', '', l) for l in lines]
        content = "\n".join(cleaned_lines)
        return f'<div class="callout-note"><div class="callout-title">📌 NOTE</div><p>{content}</p></div>\n'
    
    md_text = re.sub(alert_pattern, replace_alert, md_text)

    # Initialize markdown-it
    md = MarkdownIt("commonmark").enable("table")
    body_html = md.render(md_text)

    # Transform mermaid code blocks to <div class="mermaid">
    def clean_mermaid(match):
        code_content = match.group(1)
        code_content = html.unescape(code_content)
        return f'<div class="mermaid-container"><div class="mermaid">\n{code_content}\n</div></div>'

    body_html = re.sub(
        r'<pre><code class="language-mermaid">([\s\S]*?)</code></pre>',
        clean_mermaid,
        body_html
    )

    # Style scorecard text block
    scorecard_match = re.search(r'<pre><code class="language-text">(=+\s*\n🌮 NACHO FLOW AUTONOMOUS RUN SCORECARD[\s\S]*?)</code></pre>', body_html)
    if scorecard_match:
        raw_scorecard = scorecard_match.group(1)
        styled_scorecard = f'''
        <div class="terminal-card">
            <div class="terminal-bar">
                <span class="dot dot-red"></span>
                <span class="dot dot-yellow"></span>
                <span class="dot dot-green"></span>
                <span class="terminal-title">telemetry@nacho-flow: ~/benchmark/blackjack</span>
            </div>
            <pre class="terminal-content"><code>{raw_scorecard}</code></pre>
        </div>
        '''
        body_html = body_html.replace(scorecard_match.group(0), styled_scorecard)

    # Full HTML Template with print-optimized typography
    full_html = f'''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <title>The Autonomous Agent Runtime Whitepaper - Nacho Flow</title>
    <script src="https://cdn.jsdelivr.net/npm/mermaid@10/dist/mermaid.min.js"></script>
    <script>
        document.addEventListener("DOMContentLoaded", function() {{
            mermaid.initialize({{
                startOnLoad: true,
                theme: "neutral",
                securityLevel: "loose",
                flowchart: {{ useMaxWidth: true, htmlLabels: true, curve: "basis" }},
                sequence: {{ useMaxWidth: true, showSequenceNumbers: true }}
            }});
        }});
    </script>
    <style>
        @page {{
            size: A4;
            margin: 18mm 16mm 18mm 16mm;
            @bottom-right {{
                content: "Page " counter(page) " of " counter(pages);
                font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
                font-size: 8pt;
                color: #64748b;
            }}
            @bottom-left {{
                content: "Nacho Flow · Systems Whitepaper · September 2026";
                font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
                font-size: 8pt;
                color: #64748b;
            }}
        }}

        *, *::before, *::after {{
            box-sizing: border-box;
        }}

        body {{
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            font-size: 9.8pt;
            line-height: 1.55;
            color: #1e293b;
            background-color: #ffffff;
            margin: 0;
            padding: 0;
        }}

        /* Document Header / Cover Banner */
        .paper-header {{
            border-bottom: 2px solid #0f172a;
            padding-bottom: 14px;
            margin-bottom: 22px;
        }}

        .paper-badge {{
            display: inline-block;
            background: linear-gradient(135deg, #d97706, #b45309);
            color: #ffffff;
            font-size: 7.5pt;
            font-weight: 800;
            letter-spacing: 0.1em;
            text-transform: uppercase;
            padding: 3px 8px;
            border-radius: 4px;
            margin-bottom: 8px;
        }}

        h1 {{
            font-size: 19pt;
            font-weight: 800;
            line-height: 1.25;
            color: #0f172a;
            margin: 0 0 8px 0;
            letter-spacing: -0.02em;
        }}

        h2.subtitle {{
            font-size: 11pt;
            font-weight: 500;
            line-height: 1.4;
            color: #475569;
            margin: 0 0 14px 0;
        }}

        .meta-grid {{
            display: grid;
            grid-template-columns: repeat(4, 1fr);
            gap: 10px;
            background: #f8fafc;
            border: 1px solid #e2e8f0;
            border-radius: 6px;
            padding: 10px 12px;
            font-size: 8pt;
            margin-top: 10px;
        }}

        .meta-item strong {{
            display: block;
            color: #0f172a;
            font-size: 7.5pt;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 2px;
        }}

        .meta-item span {{
            color: #334155;
        }}

        /* Section Headings */
        h2 {{
            font-size: 13.5pt;
            font-weight: 750;
            color: #0f172a;
            border-bottom: 1px solid #e2e8f0;
            padding-bottom: 4px;
            margin: 22px 0 10px 0;
            page-break-after: avoid;
        }}

        h3 {{
            font-size: 10.5pt;
            font-weight: 700;
            color: #1e293b;
            margin: 14px 0 6px 0;
            page-break-after: avoid;
        }}

        p {{
            margin: 0 0 10px 0;
            text-align: justify;
        }}

        ul, ol {{
            margin: 0 0 10px 0;
            padding-left: 20px;
        }}

        li {{
            margin-bottom: 4px;
        }}

        strong {{
            color: #0f172a;
            font-weight: 650;
        }}

        /* Tables */
        table {{
            width: 100%;
            border-collapse: collapse;
            font-size: 8.5pt;
            margin: 12px 0 16px 0;
            page-break-inside: avoid;
            border: 1px solid #cbd5e1;
            border-radius: 4px;
            overflow: hidden;
        }}

        thead th {{
            background: #0f172a;
            color: #f8fafc;
            font-weight: 700;
            text-align: left;
            padding: 7px 9px;
            border: 1px solid #1e293b;
        }}

        tbody td {{
            padding: 6px 9px;
            border: 1px solid #e2e8f0;
            vertical-align: middle;
        }}

        tbody tr:nth-child(even) {{
            background-color: #f8fafc;
        }}

        /* Code Blocks */
        pre {{
            background: #090d16;
            color: #e2e8f0;
            padding: 10px 12px;
            border-radius: 6px;
            font-family: "Consolas", "Courier New", monospace;
            font-size: 8pt;
            line-height: 1.45;
            overflow-x: auto;
            margin: 10px 0 14px 0;
            page-break-inside: avoid;
            border: 1px solid #1e293b;
        }}

        code {{
            font-family: "Consolas", "Courier New", monospace;
            font-size: 8.5pt;
            background: #f1f5f9;
            color: #0f172a;
            padding: 1px 4px;
            border-radius: 3px;
        }}

        pre code {{
            background: transparent;
            color: inherit;
            padding: 0;
            border-radius: 0;
            font-size: 8pt;
        }}

        /* Terminal Window Card */
        .terminal-card {{
            background: #090d16;
            border: 1px solid #334155;
            border-radius: 6px;
            margin: 12px 0 16px 0;
            overflow: hidden;
            page-break-inside: avoid;
        }}

        .terminal-bar {{
            background: #1e293b;
            padding: 6px 10px;
            display: flex;
            align-items: center;
            border-bottom: 1px solid #334155;
        }}

        .dot {{
            display: inline-block;
            width: 8px;
            height: 8px;
            border-radius: 50%;
            margin-right: 5px;
        }}
        .dot-red {{ background: #ef4444; }}
        .dot-yellow {{ background: #f59e0b; }}
        .dot-green {{ background: #10b981; }}

        .terminal-title {{
            color: #94a3b8;
            font-size: 7.5pt;
            font-family: monospace;
            margin-left: 8px;
        }}

        .terminal-content {{
            margin: 0 !important;
            padding: 10px 14px !important;
            border: none !important;
            background: transparent !important;
            color: #f1f5f9 !important;
        }}

        /* Mermaid Container */
        .mermaid-container {{
            background: #ffffff;
            border: 1px solid #cbd5e1;
            border-radius: 6px;
            padding: 12px;
            margin: 12px 0 16px 0;
            text-align: center;
            page-break-inside: avoid;
        }}

        .mermaid svg {{
            max-width: 100% !important;
            height: auto !important;
        }}

        /* Callout Notes */
        .callout-note {{
            background: #fffbeb;
            border-left: 4px solid #d97706;
            border-radius: 0 6px 6px 0;
            padding: 10px 12px;
            margin: 12px 0 14px 0;
            font-size: 9pt;
            page-break-inside: avoid;
        }}

        .callout-title {{
            font-weight: 750;
            color: #92400e;
            font-size: 8pt;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 4px;
        }}

        .callout-note p {{
            margin: 0;
            color: #78350f;
        }}

        /* Section Breaks */
        .page-break {{
            page-break-before: always;
        }}

        hr {{
            border: 0;
            border-top: 1px solid #e2e8f0;
            margin: 18px 0;
        }}

        /* Link Styling */
        a {{
            color: #2563eb;
            text-decoration: none;
        }}
    </style>
</head>
<body>
    <div class="paper-header">
        <span class="paper-badge">Systems Architecture & Empirical Benchmark Report</span>
        <h1>Why Autonomous Coding Agents Fail in Production</h1>
        <h2 class="subtitle">Mitigating Context Snowballs, Code Churn, and Protocol Fragility with an Active Execution Runtime (The 44-Cent Paradigm)</h2>
        
        <div class="meta-grid">
            <div class="meta-item">
                <strong>Author</strong>
                <span>@dixieflatline76 (Nacho Flow)</span>
            </div>
            <div class="meta-item">
                <strong>Target Stack</strong>
                <span>Nacho Flow v1.4.1 · Zoo Code v3.84 · Go 1.26</span>
            </div>
            <div class="meta-item">
                <strong>Publication Date</strong>
                <span>September 2026</span>
            </div>
            <div class="meta-item">
                <strong>Empirical Benchmark</strong>
                <span>64-Turn Autonomous Blackjack Run vs Frontier Control</span>
            </div>
        </div>
    </div>

    {body_html}

</body>
</html>'''

    tmp_html = os.path.join(repo_root, "scripts", "whitepaper_print.html")
    with open(tmp_html, "w", encoding="utf-8") as f:
        f.write(full_html)
    print(f"Generated standalone HTML: {tmp_html}")

    # Output PDF destinations
    dest_repo_pdf = os.path.join(repo_root, "docs", "NACHO_FLOW_AUTONOMOUS_AGENT_RUNTIME_WHITEPAPER.pdf")
    dest_site_pdf = os.path.join(repo_root, "site", "docs", "NACHO_FLOW_AUTONOMOUS_AGENT_RUNTIME_WHITEPAPER.pdf")
    
    # Artifacts path
    artifacts_dir = r"C:\Users\karlk\.gemini\antigravity-ide\brain\b1402da8-4586-4f87-871b-3566338b03c1"
    dest_artifact_pdf = os.path.join(artifacts_dir, "NACHO_FLOW_AUTONOMOUS_AGENT_RUNTIME_WHITEPAPER.pdf")

    edge_bin = r"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"
    if not os.path.exists(edge_bin):
        raise FileNotFoundError(f"Microsoft Edge binary not found at {edge_bin}")

    cmd = [
        edge_bin,
        "--headless",
        "--disable-gpu",
        "--run-all-compositor-stages-before-draw",
        "--virtual-time-budget=6000",
        "--no-pdf-header-footer",
        f"--print-to-pdf={dest_repo_pdf}",
        tmp_html
    ]

    print("Compiling PDF with Microsoft Edge headless engine...")
    subprocess.run(cmd, check=True)

    if os.path.exists(dest_repo_pdf):
        size_kb = os.path.getsize(dest_repo_pdf) / 1024
        print(f"Successfully created: {dest_repo_pdf} ({size_kb:.1f} KB)")
        
        # Copy to site docs
        shutil.copy2(dest_repo_pdf, dest_site_pdf)
        print(f"Mirrored to site docs: {dest_site_pdf}")

        # Copy to artifacts
        if os.path.exists(artifacts_dir):
            shutil.copy2(dest_repo_pdf, dest_artifact_pdf)
            print(f"Mirrored to artifacts: {dest_artifact_pdf}")
    else:
        raise RuntimeError("PDF generation failed.")

if __name__ == "__main__":
    build_pdf()
