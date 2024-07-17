# SDCM - sort dicom files into folders

Usage:

```bash
sdcm <input folder> <output folder>
```
The output folder should exist, but be empty. This program will chicken out if it finds already a folder 'input' in the output.

### Install on MacOS

Download the sdcm executable that matches your platform. Copy the file to a folder in your path (e.g. /usr/local/bin).


```bash
# Intel-based mac (amd64)
wget -qO- https://github.com/mmiv-center/Research-Information-System/raw/master/components/Workflow-Image-AI/build/macos-amd64/sdcm > /usr/local/bin/sdcm
chmod +x /usr/local/bin/sdcm
```

```bash
# Silicon-based mac (arm64)
wget -qO- https://github.com/mmiv-center/Research-Information-System/raw/master/components/Workflow-Image-AI/build/macos-arm64/ror > /usr/local/bin/sdcm
chmod +x /usr/local/bin/sdcm
```


### Install on Windows

Download the ror.exe. Copy the program to your program files folder. The line below will only work in the cmd terminal and with administrator rights. If you don't have those rights copy the executable into one of your own directories and add that to the PATH environment variable in system settings.

```bash
wget -qO- https://github.com/mmiv-center/Research-Information-System/raw/master/components/Workflow-Image-AI/build/windows-amd64/ror.exe > %ProgramFiles%/ror.exe
```

### Install on Linux

Download the executable. Copy the file to a folder like /usr/local/bin/ that is in your path.

```bash
wget -qO- https://github.com/mmiv-center/Research-Information-System/raw/master/components/Workflow-Image-AI/build/linux-amd64/ror > /usr/local/bin/ror
chmod +x /usr/local/bin/ror
```
