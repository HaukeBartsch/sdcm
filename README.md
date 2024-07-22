# SDCM - sort dicom files into folders

Usage:

```bash
sdcm -verbose -method link <input folder> <output folder>
```

The output folder should not exist, or be empty. This program will chicken out if the output folder already contains files.

The output folder contains a directory tree with studies, series and (symbolic) links to the DICOM images:

```bash
> sdcm -verbose -method link /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI /tmp/bbb
Parse /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI...
⣯ 244600 [988 files / s] P1010 S1308 S1398
done in 4m7.765658167s 
✓ sorted 244617 files into /tmp/bbb [1317 non-DICOM files ignored]

> tree -L 3 /tmp/bbb
/tmp/bbb
├── LIDC-IDRI-0001_
│   ├── 20000101__1.3.6.1.4.1.14519.5.2.1.6279.6001.175012972118199124641098335511
│   │   └── 3000923__1.3.6.1.4.1.14519.5.2.1.6279.6001.141365756818074696859567662357
│   └── 20000101__1.3.6.1.4.1.14519.5.2.1.6279.6001.298806137288633453246975630178
│       └── 3000566__1.3.6.1.4.1.14519.5.2.1.6279.6001.179049373636438705059720603192
...
```

## Timing

The following table compares the processing speed of sdcm and Horos 4.0.1 (on MacBook Air 13, M2 arm64) for 244,617 DICOM files (LIDC-IDRI dataset from an external SSD):

| Program | Task | Timing |
| --- | --- | --- |
| Horos v4.01 | process 244,617 DICOM and 1,300 non-DICOM files | 7m50s |
| sdcm v0.0.2 | process 244,617 DICOM and 1,300 non-DICOM files  | 4m12s |

In this test Horos was asked to only "link" to the input folder. About 970 images per second can be processed by sdcm. Using "-method copy" approximately 200 files per second are processed on the same machine.


### Details

Writing to disk is usually the slowest part of sorting DICOM files. To speed this up the '-method link' option will not copy the content of the input files. Instead a symbolic link file (smaller) that points to the each input file is created. In order to obtain a copy of the files you need to dereference each symbolic link. The 'cp' program has an option '-L' that follows symbolic links with:

```bash
cp -Lr <output folder>/input/<patient>/<study>/<series> /somewhere/else/
```

The default option '-method copy' is slower but generates a physical copy of files in the output folder. If you are are only interested in a single series use '-method link' followed by 'cp -L'. 

Warning: Scanning non-DICOM files takes a lot of time. sdcm uses a heuristic based on filenames. It assumes that DICOM files either do not have an extension or have the ".dcm" extension. All other files are ignored. This implies that sdcm will ignore files with an extension like ".dcm.bak".

#### Output folder structure

The default output folder structure combines patient, study and series level information. You can specify a simplier output format using the "-folder" option.

Default folders:

```bash
sdcm -verbose \
     -method link \
     -folder "{PatientID}_{PatientName}/{StudyDate}_{StudyTime}_{StudyInstanceUID}/{SeriesNumber}_{SeriesDescription}_{SeriesInstanceUID}/{Modality}_{SOPInstanceUID}.dcm" \
     <input folder> <output folder>
```

Simple folder:

```bash
sdcm -verbose \
     -method link \
     -folder "{PatientID}/{SeriesNumber}_{SeriesDescription}/{Modality}_{counter}.dcm" \
     <input folder> <output folder>
```

BIDS like folder:

```bash
sdcm -verbose \
     -method link \
     -folder "ProjectX/sub-{PatientID}/ses-{StudyDate}_{StudyTime}/{SeriesNumber}_{SeriesDescription}/{Modality}_{counter}.dcm" \
     <input folder> <output folder>
```


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
