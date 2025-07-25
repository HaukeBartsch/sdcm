# SDCM - sort dicom files into folders

![example run](https://github.com/HaukeBartsch/sdcm/raw/main/images/sdcm.gif)

Usage:

```bash
sdcm -method link (input folder) [(input folder N) ...] (output folder)
```

The output folder should not exist, or be empty (see option -brave).

Here an example processing run with a generated output directory tree with studies, series and (symbolic) links to the DICOM images:

```bash
> sdcm -method link /Volumes/T7/data/LIDC-IDRI/LIDC-IDRI /tmp/bbb
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

The default (option '-method copy') is slower but generates a physical copy of files in the output folder. If you are only interested in a single series use '-method link' followed by 'cp -L'.

> [!NOTE]
> Warning: Scanning large non-DICOM files takes a lot of time until it fails. To reduce that scantime sdcm uses a heuristic based on filenames. It assumes that DICOM files either do not have an extension or have the ".dcm" extension. All other files are ignored. This implies that sdcm will ignore files with an extension like ".dcm.bak". You can disable this behavior, scan all files with option -thorrough.


During processing the command line will show:

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

The default output folder structure combines patient, study and series level information. You can specify an output format using the "-folder" or "-format" option.

Default (made explicit):

```bash
sdcm -method link \
     -folder "{PatientID}_{PatientName}/{StudyDate}_{StudyTime}_{StudyInstanceUID}/{SeriesNumber}_{SeriesDescription}_{SeriesInstanceUID}/{Modality}_{SOPInstanceUID}.dcm" \
     <input folder> <output folder>
```

Simple folders:

```bash
sdcm -method link \
     -folder "{PatientID}/{SeriesNumber}_{SeriesDescription}/{Modality}_{counter}.dcm" \
     <input folder> <output folder>
```

BIDS-like folders:

```bash
sdcm -method link \
     -folder "ProjectX/sub-{PatientID}/ses-{StudyDate}_{StudyTime}/{SeriesNumber}_{SeriesDescription}/{Modality}_{counter}.dcm" \
     <input folder> <output folder>
```

Only create folders (-method dirs_only) and only a single level with series descriptions:

```bash
sdcm -method dirs_only \
     -format "{SeriesDescription}" \
     <input folder> <output folder>
```

The folder option for future runs of sdcm can also be set as an environment variable SDCM_FOLDER_PATH.

```bash
SDCM_FOLDER_PATH="{PatientID}/{StudyDate}/{SeriesNumber}_{SeriesDescription}/{Modality}_{counter}.dcm"
sdcm -method link <input folder> <output folder>
```

Store a folder path in an external text file. Such a file can be used on the command line if the value of '-folder' starts with a '@'-character (e.g. '-folder @my_folder_options_file.txt').

```bash
# Example format path file for sdcm
# Text after a '#' character is ignored. Spaces are also ignored.
# Uses empty strings if tags have no value or do not exist.
# Use this template (save as default_format) with:
#     sdcm -format @default_format <input folder> <output folder>

{PatientID}_{PatientName}/
	{StudyDate}_{StudyTime}/
		{SeriesNumber}_{SeriesDescription}/
			{Modality}_{SOPInstanceUID}.dcm
```

### Filter for specific files

By specifying a regular expression for a DICOM tag you can restrict the output to matching files only. For example "{Modality==MR}" will restrict the output to files with the modality tag "MR".


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

Test the installation by running the following command (use "sdcm.exe" on Windows):

```bash
sdcm --help
```

This should print the help message:

```
NAME
        sdcm - sort DICOM files into folders

USAGE
        sdcm (input folder) [(input folder N) ...] (output folder)

DESCRIPTION
        sdcm copies DICOM files from one directory to another. The output directory tree structure is user defined and based on DICOM meta-data.
        Additionally to named DICOM tags a numeric '{counter}' variable can be used. The argument to option 'folder' will be interpreted
        as a filename if it starts with an '@'-character. The file may contain the folder path as text.

                # Example format path file for sdcm
                # Text after a '#' character is ignored. Spaces are also ignored.
                # Uses empty strings if tags have no value or do not exist.
                # Use this template with (save as default_format):
                #     sdcm -format @default_format (input folder) (output folder)

                {PatientID}_{PatientName}/
                        {StudyDate}_{StudyTime}/
                                {SeriesNumber}_{SeriesDescription}/
                                        {Modality}_{SOPInstanceUID}.dcm

        To filter for specific DICOM files add a regular expression to the DICOM tag after '=='.

        Example:
                {Modality==(MR|CT)}

OPTIONS
  -brave
        write files even if the output folder already exists and it is not empty
  -cpus
        number of worker threads used for processing (default 16)
  -debug
        print verbose and add messages for skipped files
  -folder
        specify the requested output folder path
         (default {PatientID}_{PatientName}/{StudyDate}_{StudyTime}/{SeriesNumber}_{SeriesDescription}/{Modality}_{SOPInstanceUID}.dcm)
  -format
        same as -folder
         (default {PatientID}_{PatientName}/{StudyDate}_{StudyTime}/{SeriesNumber}_{SeriesDescription}/{Modality}_{SOPInstanceUID}.dcm)
  -method
        create either symbolic links (faster) or copy files. If dirs_only is used no files are created [copy|link|dirs_only] (default copy)
  -preserve
        preserves the timestamp if called with '-preserve timestamp'. This option only works together with '-method copy'
  -quiet
        do not print anything
  -thorough
        do not filter files by extension, process all files (slower)
  -verbose
        print more verbose output
  -version
        print the version number

ENVIRONMENT
        The following environment variables affect the execution of sdcm:

        SDCM_FOLDER_PATH
                The default value for option -folder.

```

### Notes

Shells like zsh can understand the help (--help) page produced by programs like sdcm. You can teach your shell the options of sdcm with


```bash
# store in ~/.zshrc
compdef _gnu_generic sdcm
```

If you type 'sdcm -<TAB>' the shell can show you the options available. Add the above compdef line to your ~/.zshrc.

A list of skipped or non-DICOM files is generated in debug mode. To create a transcript of file that are ignored use

```bash
sdcm -debug <input> <output> 2>log.txt
cat log.txt
```


### Failure modes

A common error is to have insufficient permissions for all or some of the folders. Make sure you are
allowed to read from all input files and directories and that you have permissions to write into the
output directory.

You get too many output directories? In this case there could have been an error with the pseudonymization
procedure of your DICOM data. Fix that anonymization process and start again.

> [!NOTE]
> Individual DICOM files depend on each others meta-data. Id values have to agree on the study and series level to correctly encode that images belong to a volume. A study is usually a single scanning event, a series is a subset of the files from a study, for example all files that belong to the same volume. A pseudonymization procedure can try to change these study and series level id's. If it tries to do this it must ensure that the generated ids follow the same logic of study and series. All DICOM files that belong to the same study need the same id (StudyInstanceUID). All DICOM files that belong to the same series/volume need the same new series id (SeriesInstanceUID). Using your anonymizer wrongly - e.g. performing pseudonymization of ids individually on each DICOM file will destroy the assignment of individual images to series and studies. Such a broken collection of DICOM files results in many folders, one for each image.

The output folder does not contain DICOM files? Using "-method link" the output folder will contain pointer files (symbolic links) only. Only on the same system you can use these output files directly. If you are planning to send the DICOM files to another system use "-method copy". Or, link and resolves the symbolic links (e.g. 'cp -Lr' above).

Copies of DICOM files can appear in the input directories. For example a file A.dcm might have a A.dcm.bak file next to it with some tags changed. Copies of files can also appear if you use a '-folder' option that generates the same name for different input files. SDCM will try to resolve this by making the output file names unique (adding _001 etc.). You can show a message with -verbose for such copies.