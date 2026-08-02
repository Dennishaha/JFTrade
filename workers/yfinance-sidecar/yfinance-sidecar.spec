# PyInstaller onedir build specification for the local yfinance sidecar.

from pathlib import Path
import os

from PyInstaller.utils.hooks import collect_all


project_dir = Path(SPECPATH).resolve()
src_dir = project_dir / "src"
datas = []
binaries = []
hiddenimports = []

for package in ("curl_cffi", "yfinance"):
    package_datas, package_binaries, package_hiddenimports = collect_all(package)
    datas.extend(package_datas)
    binaries.extend(package_binaries)
    hiddenimports.extend(package_hiddenimports)

hiddenimports.extend([
    "uvicorn.lifespan.on",
    "uvicorn.loops.asyncio",
    "uvicorn.protocols.http.h11_impl",
])
excluded_uvicorn_modules = [
    "uvloop",
    "uvicorn.loops.auto",
    "uvicorn.loops.uvloop",
    "uvicorn.protocols.http.auto",
    "uvicorn.protocols.http.httptools_impl",
    "uvicorn.protocols.websockets",
]
binary_name = os.environ.get("JFTRADE_YFINANCE_BINARY_NAME", "yfinance-sidecar")


a = Analysis(
    [str(src_dir / "yfinance_sidecar" / "__main__.py")],
    pathex=[str(src_dir)],
    binaries=binaries,
    datas=datas,
    hiddenimports=hiddenimports,
    hookspath=[],
    hooksconfig={},
    runtime_hooks=[],
    excludes=excluded_uvicorn_modules,
    noarchive=False,
)
pyz = PYZ(a.pure)
exe = EXE(
    pyz,
    a.scripts,
    [],
    exclude_binaries=True,
    name=binary_name,
    debug=False,
    bootloader_ignore_signals=False,
    strip=False,
    upx=False,
    console=True,
    disable_windowed_traceback=False,
)
coll = COLLECT(
    exe,
    a.binaries,
    a.datas,
    strip=False,
    upx=False,
    name=binary_name,
)
