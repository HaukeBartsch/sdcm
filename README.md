# SDCM - sort dicom files into folders

![example run](https://github.com/HaukeBartsch/sdcm/raw/main/images/sdcm.gif)

Usage:

```bash
sdcm -verbose -method link <input folder> <output folder>
```

The output folder should not exist, or be empty.

Here an example processing run with a generated output directory tree with studies, series and (symbolic) links to the DICOM images:

```bash
> sdcm -verbose -method link /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI /tmp/bbb
Parse /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI...
⣯ 244,600 [988 files / s] P1010 S1308 S1398
done in 4m7.765658167s 
✓ sorted 244,617 files into /tmp/bbb [1,317 non-DICOM files ignored]

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

The following table compares the processing speeds of sdcm and some other tools (on MacBook Air 13, M2 arm64) for 244,617 DICOM files (LIDC-IDRI dataset from an external SSD):

| Program | Task | Timing |
| --- | --- | --- |
| Horos v4.01 | process 244,617 DICOM and 1,317 non-DICOM files | 7m50s |
| [Python/pydicom](https://github.com/HaukeBartsch/sort_dicom_files) | process 244,617 DICOM and 1,317 non-DICOM files | 10m17s |
| [bash/dcmtk](https://github.com/HaukeBartsch/sort_dicom_files) | process 244,617 DICOM and 1,317 non-DICOM files | >1h |
| sdcm v0.0.2 | process 244,617 DICOM and 1,317 non-DICOM files | 4m12s |

In this test Horos was asked to "link" to the input folder. The python script was started with the '-symlink' flag. About 970 images per second can be processed by sdcm. Using "-method copy" approximately 200 files per second are processed on the same machine. 

> [!NOTE]
> The bash option is by far the worst-case scenario, not because of bash but because DICOM tags are extracted using repeated calls with "dcmdump". This could be improved by using dcm2json and pulling values using jq (left to the reader).


## Details

Writing to disk is usually the slowest part of sorting DICOM files. To speed this up the '-method link' option will not copy the content of the input files. Instead symbolic link files (smaller) that points to each input file are created. In order to obtain a copy of the files you need to dereference each symbolic link. The 'cp' program has an option '-L' that follows symbolic links with:

```bash
cp -Lr <output folder>/input/<patient>/<study>/<series> /somewhere/else/
```

The default (option '-method copy') is slower but generates a physical copy of files in the output folder. If you are are only interested in a single series use '-method link' followed by 'cp -L'. 

> [!NOTE]
> Warning: Scanning large non-DICOM files takes a lot of time until it fails. To reduce that scantime sdcm uses a heuristic based on filenames. It assumes that DICOM files either do not have an extension or have the ".dcm" extension. All other files are ignored. This implies that sdcm will ignore files with an extension like ".dcm.bak".


During processing with '-verbose' the command line will show:

```bash
⢿ 42,982 [118 files / s] P 12,102 S 12,111 S 12,374 [S 134,118]
  |       |              |        |        |         |
  Number of DICOM files  |        |        |         |
          Overall speed of processing      |         |
                         Number of patients          |
                                  Number of studies  |
                                           Number of series
                                                     Number of skipped files (non-DICOM)
```


## Output folder structure

The default output folder structure combines patient, study and series level information. You can specify a simpler output format using the "-folder" option.

Default folders:

```bash
sdcm -verbose \
     -method link \
     -folder "{PatientID}_{PatientName}/{StudyDate}_{StudyTime}_{StudyInstanceUID}/{SeriesNumber}_{SeriesDescription}_{SeriesInstanceUID}/{Modality}_{SOPInstanceUID}.dcm" \
     <input folder> <output folder>
```

Simple folders:

```bash
sdcm -verbose \
     -method link \
     -folder "{PatientID}/{SeriesNumber}_{SeriesDescription}/{Modality}_{counter}.dcm" \
     <input folder> <output folder>
```

BIDS-like folders:

```bash
sdcm -verbose \
     -method link \
     -folder "ProjectX/sub-{PatientID}/ses-{StudyDate}_{StudyTime}/{SeriesNumber}_{SeriesDescription}/{Modality}_{counter}.dcm" \
     <input folder> <output folder>
```

The folder option can also be set as an environment variable SDCM_FOLDER_PATH.

```bash
SDCM_FOLDER_PATH="{PatientID}/{StudyDate}/{SeriesNumber}_{SeriesDescription}/{Modality}_{counter}.dcm" 
sdcm -verbose -method link <input folder> <output folder>
```

You can store a folder path in an external text file. Such a file can be used on the command line if the value of '-folder' starts with a '@'-character (e.g. '-folder @my_folder_options_file.txt').

```bash
# Example format path file for sdcm
# Text after a '#' character is ignored. Spaces are also ignored.
# Uses empty strings if tags have no value or do not exist.
# Use this template with:
#     sdcm -format @default_format <input folder> <output folder>

{PatientID}_{PatientName}/			
	{StudyDate}_{StudyTime}/
		{SeriesNumber}_{SeriesDescription}/
			{Modality}_{SOPInstanceUID}.dcm
```


### Install on MacOS

Download the sdcm executable that matches your platform. Copy the file (statically linked executable) to a folder in your path (e.g. /usr/local/bin). The instructions below work if you have access to 'wget' (install on MacOS with 'brew', use 'sudo' if you do not have permissions to write to /usr/local/bin/).

Intel-based mac (amd64)

```bash
sudo wget -qO- https://github.com/HaukeBartsch/sdcm/raw/main/build/linux-amd64/sdcm > /usr/local/bin/sdcm
chmod +x /usr/local/bin/sdcm
```

Silicon-based mac (arm64)

```bash
sudo wget -qO- https://github.com/HaukeBartsch/sdcm/raw/main/build/macos-arm64/sdcm > /usr/local/bin/sdcm
chmod +x /usr/local/bin/sdcm
```


### Install on Windows

Download the sdcm.exe. Copy the program to your program files folder. The line below will only work in the cmd terminal and with administrator rights. If you don't have those rights copy the executable into one of your own directories and add that to the PATH environment variable in system settings.

```bash
wget -qO- https://github.com/HaukeBartsch/sdcm/raw/main/build/windows-amd64/sdcm.exe > %ProgramFiles%/sdcm.exe
```

### Install on Linux

Download the executable. Copy the file to a folder like /usr/local/bin/ that is in your path.

```bash
wget -qO- https://github.com/HaukeBartsch/sdcm/raw/main/build/linux-amd64/sdcm > /usr/local/bin/sdcm
chmod +x /usr/local/bin/sdcm
```

### Test the installation

Test the installation by running the following command:

```bash
sdcm --help
```

This should print the help message:

```
Usage of sdcm:
  -debug
    	Print verbose and add messages for skipped files
  -folder string
    	Specify the requested output folder path as a string (or file starting with '@') using the following DICOM tags:
    		{counter}, {PatientID}, {PatientName}, {StudyDate},
    		{StudyTime}, {SeriesDescription}, {SeriesNumber}, {StudyDescription},
    		{Modality}, {StudyInstanceUID}, {SeriesInstanceUID}, {SOPInstanceUID}.
    	The argument will be interpreted as a filename if it is preceeded with a '@'-character.
    	 (default "{PatientID}_{PatientName}/{StudyDate}_{StudyTime}/{SeriesNumber}_{SeriesDescription}/{Modality}_{SOPInstanceUID}.dcm")
  -method string
    	Create symbolic links (faster) or copy files [copy|link] (default "copy")
  -verbose
    	Print more verbose output
  -version
    	Print the version number
```