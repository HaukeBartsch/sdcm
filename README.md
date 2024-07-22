# SDCM - sort dicom files into folders

Usage:

```bash
sdcm -verbose -method link <input folder> <output folder>
```
The output folder should exist, but be empty. This program will chicken out if it finds a folder called 'input' in the output folder.

With the above options the output folder will contain a directory 'input' with studies, series and links to the DICOM images in the input folder:

```bash
<output folder>
└── input
    ├── LIDC-IDRI-0001_
    │   ├── 20000101__1.3.6.1.4.1.14519.5.2.1.6279.6001.175012972118199124641098335511
    │   │   └── 3000923__1.3.6.1.4.1.14519.5.2.1.6279.6001.141365756818074696859567662357
    │   │       ├── DX_1.3.6.1.4.1.14519.5.2.1.6279.6001.257944242390114100388269195181.dcm -> /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI/LIDC-IDRI-0001/01-01-2000-35511/3000923-62357/000002.dcm
    │   │       └── DX_1.3.6.1.4.1.14519.5.2.1.6279.6001.307896144859643716158189196068.dcm -> /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI/LIDC-IDRI-0001/01-01-2000-35511/3000923-62357/000001.dcm
    │   └── 20000101__1.3.6.1.4.1.14519.5.2.1.6279.6001.298806137288633453246975630178
    │       └── 3000566__1.3.6.1.4.1.14519.5.2.1.6279.6001.179049373636438705059720603192
    │           ├── CT_1.3.6.1.4.1.14519.5.2.1.6279.6001.100954823835603369147775570297.dcm -> /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI/LIDC-IDRI-0001/01-01-2000-30178/3000566-03192/000122.dcm
    │           ├── CT_1.3.6.1.4.1.14519.5.2.1.6279.6001.101045044159171311719370216637.dcm -> /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI/LIDC-IDRI-0001/01-01-2000-30178/3000566-03192/000107.dcm
    │           ├── CT_1.3.6.1.4.1.14519.5.2.1.6279.6001.104640960159524969909035876745.dcm -> /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI/LIDC-IDRI-0001/01-01-2000-30178/3000566-03192/000075.dcm
    │           ├── CT_1.3.6.1.4.1.14519.5.2.1.6279.6001.104650143968793544078397221048.dcm -> /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI/LIDC-IDRI-0001/01-01-2000-30178/3000566-03192/000124.dcm
    ...
```

## Timing

Timing between sdcm and Horos 4.0.1 (on MacBook Air 13, M2 arm64) for processing of 244,617 DICOM images (LIDC-IDRI dataset of XRay and CT on external SSD):

| Program | Task | Timing |
| --- | --- | --- |
| Horos v4.01 | process 244,617 DICOM and 1,300 non-DICOM files | 7m50s |
| sdcm v0.0.2 | process 244,617 DICOM and 1,300 non-DICOM files  | 4m12s |

In this test Horos was asked to only "link" in the input folder. About 1,000 images per second can be processed by sdcm.

### Details

Writing to disk is usually the slowest part of sorting DICOM files. To speed this up the '-method link' option will not copy the input files. Instead a symbolic link that points to the input file is created. In order to obtain a 'real' copy of the files dereference the symbolic links. The 'cp' program has an option '-L' that follows symbolic links:

```bash
cp -Lr <output folder>/input/<patient>/<study>/<series> /somewhere/else/
```

By default the option '-method copy' is used which is slower but copies files to the output folder. If you are are only interested in a single series you should use '-method link' followed by 'cp -L'. 

Warning: Scanning non-DICOM files takes a lot of time. We use a heuristic here that a DICOM files either does not have an extension or has the ".dcm" extension. All other files will be ignored.


### Install on MacOS

Download the sdcm executable that matches your platform. Copy the file (statically linked executable) to a folder in your path (e.g. /usr/local/bin).


```bash
# Intel-based mac (amd64)
wget -qO- https://github.com/HaukeBartsch/sdcm/raw/main/build/linux-amd64/sdcm > /usr/local/bin/sdcm
chmod +x /usr/local/bin/sdcm
```

```bash
# Silicon-based mac (arm64)
wget -qO- https://github.com/HaukeBartsch/sdcm/raw/main/build/macos-arm64/sdcm > /usr/local/bin/sdcm
chmod +x /usr/local/bin/sdcm
```


### Install on Windows

Download the ror.exe. Copy the program to your program files folder. The line below will only work in the cmd terminal and with administrator rights. If you don't have those rights copy the executable into one of your own directories and add that to the PATH environment variable in system settings.

```bash
wget -qO- https://github.com/HaukeBartsch/sdcm/raw/main/build/windows-amd64/sdcm.exe > %ProgramFiles%/sdcm.exe
```

### Install on Linux

Download the executable. Copy the file to a folder like /usr/local/bin/ that is in your path.

```bash
wget -qO- https://github.com/HaukeBartsch/sdcm/raw/main/build/linux-amd64/sdcm > /usr/local/bin/sdcm
chmod +x /usr/local/bin/sdcm
```
